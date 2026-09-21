// Package api exposes the management, OpenAI-compatible and prediction HTTP APIs.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/donvito/modelserver/internal/models"
	rt "github.com/donvito/modelserver/internal/runtime"
	"github.com/donvito/modelserver/internal/service"
)

type Server struct {
	svc     *service.Service
	apiKeys []string
	ui      fs.FS // nil in headless mode
	logger  *slog.Logger
}

// Options configures the HTTP server.
type Options struct {
	APIKeys []string
	// UI is the built dashboard; nil disables it (headless).
	UI     fs.FS
	Logger *slog.Logger
}

func New(svc *service.Service, opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Server{svc: svc, apiKeys: opts.APIKeys, ui: opts.UI, logger: opts.Logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Management API
	mux.HandleFunc("GET /api/models", s.listModels)
	mux.HandleFunc("POST /api/models", s.createModel)
	mux.HandleFunc("GET /api/models/{id}", s.getModel)
	mux.HandleFunc("PATCH /api/models/{id}", s.updateModel)
	mux.HandleFunc("DELETE /api/models/{id}", s.deleteModel)
	mux.HandleFunc("POST /api/models/{id}/load", s.loadModel)
	mux.HandleFunc("POST /api/models/{id}/unload", s.unloadModel)
	mux.HandleFunc("POST /api/models/{id}/restart", s.restartModel)
	mux.HandleFunc("GET /api/models/{id}/status", s.statusModel)
	mux.HandleFunc("GET /api/models/{id}/logs", s.logsModel)
	mux.HandleFunc("GET /api/models/{id}/logs/stream", s.streamLogs)
	mux.HandleFunc("GET /api/system", s.system)
	mux.HandleFunc("GET /api/system/health", s.health)
	mux.HandleFunc("GET /api/runtimes", s.runtimes)
	mux.HandleFunc("GET /api/runtimes/{runtime}/logs", s.logsRuntime)

	// OpenAI-compatible API
	mux.HandleFunc("GET /v1/models", s.openAIModels)
	mux.HandleFunc("POST /v1/chat/completions", s.openAIProxy)
	mux.HandleFunc("POST /v1/completions", s.openAIProxy)
	mux.HandleFunc("POST /v1/embeddings", s.openAIProxy)

	// Generic prediction API
	mux.HandleFunc("POST /v1/models/{model}/predict", s.predict)

	if s.ui != nil {
		mux.Handle("/", spaHandler(s.ui))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				writeJSON(w, http.StatusOK, map[string]any{"name": "modelserver", "version": service.Version, "ui": false, "docs": "/api/system"})
				return
			}
			writeError(w, http.StatusNotFound, "not found", "not_found")
		})
	}

	var h http.Handler = mux
	h = s.auth(h)
	h = s.logRequests(h)
	return h
}

// --- middleware ---

func (s *Server) auth(next http.Handler) http.Handler {
	if len(s.apiKeys) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		// Health and the UI shell stay public; every API route needs a key.
		if p == "/api/system/health" || !(strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/v1/")) {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" {
			token = r.Header.Get("X-API-Key")
		}
		if token == "" {
			token = r.URL.Query().Get("api_key")
		}
		for _, k := range s.apiKeys {
			if subtle.ConstantTimeCompare([]byte(k), []byte(token)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="modelserver"`)
		writeError(w, http.StatusUnauthorized, "invalid or missing API key", "unauthorized")
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.ui != nil && !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError uses the OpenAI error envelope so both API families read the same.
func writeError(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": msg, "type": code, "code": code}})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error(), "model_not_found")
	case errors.Is(err, models.ErrNameTaken):
		writeError(w, http.StatusConflict, err.Error(), "name_taken")
	case errors.Is(err, models.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error(), "validation_error")
	case errors.Is(err, service.ErrModelBusy):
		writeError(w, http.StatusConflict, err.Error(), "model_busy")
	case errors.Is(err, rt.ErrAlreadyLoaded):
		writeError(w, http.StatusConflict, err.Error(), "already_loaded")
	case errors.Is(err, rt.ErrNotLoaded):
		writeError(w, http.StatusConflict, err.Error(), "model_not_loaded")
	case errors.Is(err, service.ErrTaskNotServing), errors.Is(err, rt.ErrUnsupportedTask):
		writeError(w, http.StatusBadRequest, err.Error(), "unsupported")
	case errors.Is(err, rt.ErrUnknownRuntime), errors.Is(err, rt.ErrUnavailable):
		writeError(w, http.StatusBadRequest, err.Error(), "runtime_error")
	default:
		writeError(w, http.StatusInternalServerError, err.Error(), "internal_error")
	}
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: invalid JSON body: %v", models.ErrValidation, err)
	}
	return nil
}

// --- management handlers ---

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	ms, err := s.svc.List(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": ms})
}

func (s *Server) createModel(w http.ResponseWriter, r *http.Request) {
	var in service.CreateInput
	if err := decodeJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	m, err := s.svc.Create(r.Context(), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	v, err := s.svc.Get(r.Context(), m.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) getModel(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) updateModel(w http.ResponseWriter, r *http.Request) {
	var in service.UpdateInput
	if err := decodeJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	m, err := s.svc.Update(r.Context(), r.PathValue("id"), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	v, _ := s.svc.Get(r.Context(), m.ID)
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteModel(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loadModel(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		if v != nil && !errors.Is(err, rt.ErrAlreadyLoaded) {
			// Load failed: report the model view with its failure so the UI can show it.
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"message": err.Error(), "type": "load_failed", "code": "load_failed"}, "model": v})
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) unloadModel(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Unload(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) restartModel(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Restart(r.Context(), r.PathValue("id"))
	if err != nil {
		if v != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"message": err.Error(), "type": "load_failed", "code": "load_failed"}, "model": v})
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) statusModel(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Status(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": v.ID, "name": v.Name, "status": v.Live.State, "live": v.Live})
}

func (s *Server) logsRuntime(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("runtime")
	found := false
	for _, runtime := range s.svc.Runtimes().All() {
		if runtime.Name() == name {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "runtime not found", "not_found")
		return
	}
	n := 300
	if q := r.URL.Query().Get("lines"); q != "" {
		parsed, err := strconv.Atoi(q)
		if err != nil || parsed < 1 || parsed > 2000 {
			writeError(w, http.StatusBadRequest, "lines must be between 1 and 2000", "invalid_request_error")
			return
		}
		n = parsed
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtime": name, "lines": s.svc.Logs().Runtime(name).Tail(n)})
}

func (s *Server) logsModel(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	n := 200
	if q := r.URL.Query().Get("lines"); q != "" {
		if parsed, err := strconv.Atoi(q); err == nil && parsed > 0 {
			n = parsed
		}
	}
	lines := s.svc.Logs().Get(v.ID).Tail(n)
	writeJSON(w, http.StatusOK, map[string]any{"id": v.ID, "lines": lines})
}

func (s *Server) streamLogs(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported", "internal_error")
		return
	}
	buf := s.svc.Logs().Get(v.ID)
	ch, cancel := buf.Subscribe()
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(event string, v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}
	for _, l := range buf.Tail(200) {
		send("log", l)
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case l, ok := <-ch:
			if !ok {
				return
			}
			send("log", l)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.System(r.Context()))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	h, err := s.svc.Health(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, h)
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) runtimes(w http.ResponseWriter, r *http.Request) {
	var infos []rt.Info
	for _, x := range s.svc.Runtimes().All() {
		infos = append(infos, x.Info(r.Context()))
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtimes": infos})
}

// --- OpenAI-compatible handlers ---

func (s *Server) openAIModels(w http.ResponseWriter, r *http.Request) {
	ms, err := s.svc.List(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	data := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		data = append(data, map[string]any{
			"id":       m.Name,
			"object":   "model",
			"created":  m.CreatedAt.Unix(),
			"owned_by": "modelserver",
			"runtime":  m.Runtime,
			"task":     m.Task,
			"status":   m.Live.State,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// openAIProxy routes /v1/* requests to the llama-server owning the "model".
func (s *Server) openAIProxy(w http.ResponseWriter, r *http.Request) {
	body, err := peekJSON(r, 32<<20)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error(), "invalid_request_error")
		return
	}
	name, _ := body["model"].(string)
	if name == "" {
		writeError(w, http.StatusBadRequest, "'model' is required and must be the registry name of a loaded model", "invalid_request_error")
		return
	}
	_, h, err := s.svc.ProxyHandler(r.Context(), name)
	if err != nil {
		s.fail(w, err)
		return
	}
	h.ServeHTTP(w, r)
}

// --- prediction handler ---

type predictRequest struct {
	Input  any            `json:"input"`
	Inputs any            `json:"inputs"`
	Params map[string]any `json:"params"`
}

func (s *Server) predict(w http.ResponseWriter, r *http.Request) {
	var req predictRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	input := req.Input
	if input == nil {
		input = req.Inputs
	}
	if input == nil {
		writeError(w, http.StatusBadRequest, "'input' is required", "validation_error")
		return
	}
	m, resp, err := s.svc.Predict(r.Context(), r.PathValue("model"), rt.InferenceRequest{Input: input, Params: req.Params})
	if err != nil {
		if errors.Is(err, models.ErrValidation) || errors.Is(err, rt.ErrNotLoaded) || errors.Is(err, models.ErrNotFound) {
			s.fail(w, err)
			return
		}
		writeError(w, http.StatusBadGateway, err.Error(), "inference_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":   m.Name,
		"task":    m.Task,
		"runtime": m.Runtime,
		"output":  resp.Output,
		"timing":  resp.Timing,
	})
}
