package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/donvito/modelserver/internal/config"
)

// client talks to a running modelserver over its management API so that CLI
// commands operate on the live server rather than a second process.
type client struct {
	base   string
	apiKey string
	http   *http.Client
}

func newClient(cfg config.Config) *client {
	key := ""
	if len(cfg.Auth.APIKeys) > 0 {
		key = cfg.Auth.APIKeys[0]
	}
	return &client{base: "http://" + cfg.Addr(), apiKey: key, http: &http.Client{Timeout: 15 * time.Minute}}
}

func (c *client) reachable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/system/health", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type apiError struct {
	Status  int
	Message string
	Code    string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("server returned HTTP %d", e.Status)
	}
	return e.Message
}

func (c *client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach modelserver at %s: %w", c.base, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var env struct {
			Error struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &env)
		msg := env.Error.Message
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return &apiError{Status: resp.StatusCode, Message: msg, Code: env.Error.Code}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func modelPath(ref string) string { return "/api/models/" + url.PathEscape(ref) }
