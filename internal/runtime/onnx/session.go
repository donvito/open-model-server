package onnx

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/donvito/modelserver/internal/models"
	"github.com/donvito/modelserver/internal/runtime"
)

// session is a loaded ONNX model plus the pre/post-processing it needs.
type session struct {
	model  models.Model
	cfg    Config
	layout layout

	sess        *ort.DynamicAdvancedSession
	opts        *ort.SessionOptions
	inputs      []ort.InputOutputInfo
	outputs     []ort.InputOutputInfo
	inputNames  []string
	outputNames []string
	tok         *tokenizer.Tokenizer

	runMu     sync.Mutex // ONNX sessions are thread-safe, but tokenizer isn't guaranteed to be
	stateMu   sync.Mutex
	st        string
	err       string
	startedAt time.Time
}

func newSession(m models.Model, cfg Config, l layout) (*session, error) {
	s := &session{model: m, cfg: cfg, layout: l}
	inputs, outputs, err := ort.GetInputOutputInfo(l.ModelFile)
	if err != nil {
		return nil, fmt.Errorf("read ONNX model %s: %w", l.ModelFile, err)
	}
	s.inputs, s.outputs = inputs, outputs
	for _, in := range inputs {
		s.inputNames = append(s.inputNames, in.Name)
	}
	for _, out := range outputs {
		s.outputNames = append(s.outputNames, out.Name)
	}
	if m.Task != models.TaskCustom {
		if err := s.checkTextInputs(); err != nil {
			return nil, err
		}
		s.tok, err = pretrained.FromFile(l.Tokenizer)
		if err != nil {
			return nil, fmt.Errorf("load tokenizer %s: %w", l.Tokenizer, err)
		}
		// We pad ourselves per batch; disable the tokenizer's own padding and
		// truncate to the configured max length.
		s.tok.WithPadding(nil)
		s.tok.WithTruncation(&tokenizer.TruncationParams{MaxLength: cfg.maxLength(), Strategy: tokenizer.LongestFirst})
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("session options: %w", err)
	}
	if cfg.Threads > 0 {
		_ = opts.SetIntraOpNumThreads(cfg.Threads)
	}
	sess, err := ort.NewDynamicAdvancedSession(l.ModelFile, s.inputNames, s.outputNames, opts)
	if err != nil {
		opts.Destroy()
		return nil, fmt.Errorf("create ONNX session for %s: %w", l.ModelFile, err)
	}
	s.sess, s.opts = sess, opts
	return s, nil
}

var textInputs = map[string]bool{"input_ids": true, "attention_mask": true, "token_type_ids": true, "position_ids": true}

func (s *session) checkTextInputs() error {
	for _, in := range s.inputs {
		if !textInputs[in.Name] {
			return fmt.Errorf("model input %q is not a standard text input (input_ids/attention_mask/token_type_ids); register it with task=custom", in.Name)
		}
		if in.DataType != ort.TensorElementDataTypeInt64 && in.DataType != ort.TensorElementDataTypeInt32 {
			return fmt.Errorf("model input %q has type %s; expected int64 or int32", in.Name, in.DataType)
		}
	}
	return nil
}

func (s *session) setState(st, err string) {
	s.stateMu.Lock()
	s.st, s.err = st, err
	s.stateMu.Unlock()
}

func (s *session) state() string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.st
}

func (s *session) status() runtime.Status {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	st := runtime.Status{State: s.st, Error: s.err, Details: map[string]any{}}
	if s.st == models.StatusRunning {
		t := s.startedAt
		st.StartedAt = &t
		st.Details["model_file"] = s.layout.ModelFile
		st.Details["inputs"] = describe(s.inputs)
		st.Details["outputs"] = describe(s.outputs)
		if len(s.layout.Labels) > 0 {
			st.Details["labels"] = s.layout.Labels
		}
		if s.tok != nil {
			st.Details["tokenizer"] = s.layout.Tokenizer
			st.Details["max_length"] = s.cfg.maxLength()
		}
	}
	return st
}

func describe(infos []ort.InputOutputInfo) []map[string]any {
	out := make([]map[string]any, 0, len(infos))
	for _, i := range infos {
		out = append(out, map[string]any{"name": i.Name, "type": i.DataType.String(), "shape": i.Dimensions})
	}
	return out
}

func (s *session) close() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.setState(models.StatusStopped, "")
	if s.sess != nil {
		s.sess.Destroy()
		s.sess = nil
	}
	if s.opts != nil {
		s.opts.Destroy()
		s.opts = nil
	}
}

// --- inference ---

func (s *session) infer(ctx context.Context, req runtime.InferenceRequest) (runtime.InferenceResponse, error) {
	start := time.Now()
	var out any
	var err error
	switch s.model.Task {
	case models.TaskClassification:
		out, err = s.classify(req)
	case models.TaskEmbedding:
		out, err = s.embed(req)
	default:
		out, err = s.runCustom(req)
	}
	if err != nil {
		return runtime.InferenceResponse{}, err
	}
	return runtime.InferenceResponse{Output: out, Timing: runtime.Timing{LatencyMS: float64(time.Since(start).Microseconds()) / 1000}}, nil
}

// texts normalizes input into a batch of strings, remembering whether the
// caller sent a single string.
func texts(input any) ([]string, bool, error) {
	switch v := input.(type) {
	case string:
		return []string{v}, true, nil
	case []string:
		return v, false, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			str, ok := x.(string)
			if !ok {
				return nil, false, fmt.Errorf("%w: input must be a string or list of strings", models.ErrValidation)
			}
			out = append(out, str)
		}
		if len(out) == 0 {
			return nil, false, fmt.Errorf("%w: input list is empty", models.ErrValidation)
		}
		return out, false, nil
	case map[string]any:
		if t, ok := v["text"]; ok {
			return texts(t)
		}
	}
	return nil, false, fmt.Errorf("%w: input must be a string or list of strings", models.ErrValidation)
}

type batch struct {
	n, seq int
	ids    []int64
	mask   []int64
	types  []int64
}

func (s *session) tokenize(strs []string) (batch, error) {
	encs := make([]*tokenizer.Encoding, len(strs))
	seq := 0
	for i, str := range strs {
		enc, err := s.tok.EncodeSingle(str, true)
		if err != nil {
			return batch{}, fmt.Errorf("tokenize: %w", err)
		}
		encs[i] = enc
		if len(enc.Ids) > seq {
			seq = len(enc.Ids)
		}
	}
	if seq == 0 {
		seq = 1
	}
	b := batch{n: len(strs), seq: seq}
	b.ids = make([]int64, b.n*seq)
	b.mask = make([]int64, b.n*seq)
	b.types = make([]int64, b.n*seq)
	pad := int64(s.layout.PadID)
	for i, enc := range encs {
		row := b.ids[i*seq : (i+1)*seq]
		for j := range row {
			row[j] = pad
		}
		for j, id := range enc.Ids {
			row[j] = int64(id)
			b.mask[i*seq+j] = 1
			if j < len(enc.TypeIds) {
				b.types[i*seq+j] = int64(enc.TypeIds[j])
			}
		}
	}
	return b, nil
}

// runText feeds a tokenized batch through the model and returns the outputs
// (caller must Destroy them).
func (s *session) runText(b batch) ([]ort.Value, error) {
	shape := ort.NewShape(int64(b.n), int64(b.seq))
	inputs := make([]ort.Value, len(s.inputs))
	defer func() {
		for _, v := range inputs {
			if v != nil {
				v.Destroy()
			}
		}
	}()
	for i, in := range s.inputs {
		var data []int64
		switch in.Name {
		case "input_ids":
			data = b.ids
		case "attention_mask":
			data = b.mask
		case "token_type_ids":
			data = b.types
		case "position_ids":
			data = make([]int64, b.n*b.seq)
			for r := 0; r < b.n; r++ {
				for c := 0; c < b.seq; c++ {
					data[r*b.seq+c] = int64(c)
				}
			}
		}
		var t ort.Value
		var err error
		if in.DataType == ort.TensorElementDataTypeInt32 {
			d32 := make([]int32, len(data))
			for k, v := range data {
				d32[k] = int32(v)
			}
			t, err = ort.NewTensor(shape, d32)
		} else {
			t, err = ort.NewTensor(shape, data)
		}
		if err != nil {
			return nil, fmt.Errorf("create tensor %s: %w", in.Name, err)
		}
		inputs[i] = t
	}
	outputs := make([]ort.Value, len(s.outputs))
	s.runMu.Lock()
	sess := s.sess
	if sess == nil {
		s.runMu.Unlock()
		return nil, runtime.ErrNotLoaded
	}
	err := sess.Run(inputs, outputs)
	s.runMu.Unlock()
	if err != nil {
		destroyAll(outputs)
		return nil, fmt.Errorf("onnx run: %w", err)
	}
	return outputs, nil
}

func destroyAll(vs []ort.Value) {
	for _, v := range vs {
		if v != nil {
			v.Destroy()
		}
	}
}

func floatData(v ort.Value) ([]float32, ort.Shape, error) {
	switch t := v.(type) {
	case *ort.Tensor[float32]:
		return t.GetData(), t.GetShape(), nil
	case *ort.Tensor[float64]:
		d := t.GetData()
		out := make([]float32, len(d))
		for i, x := range d {
			out[i] = float32(x)
		}
		return out, t.GetShape(), nil
	}
	return nil, nil, fmt.Errorf("model output has type %s; expected float", ort.TensorElementDataType(v.DataType()))
}

// classify runs sequence classification and returns labeled probabilities.
func (s *session) classify(req runtime.InferenceRequest) (any, error) {
	strs, single, err := texts(req.Input)
	if err != nil {
		return nil, err
	}
	b, err := s.tokenize(strs)
	if err != nil {
		return nil, err
	}
	outputs, err := s.runText(b)
	if err != nil {
		return nil, err
	}
	defer destroyAll(outputs)
	logits, shape, err := floatData(outputs[0])
	if err != nil {
		return nil, err
	}
	if len(shape) != 2 || int(shape[0]) != b.n {
		return nil, fmt.Errorf("unexpected logits shape %v for batch of %d", shape, b.n)
	}
	numLabels := int(shape[1])
	topK := numLabels
	if k, ok := numberParam(req.Params, "top_k"); ok && int(k) > 0 && int(k) < numLabels {
		topK = int(k)
	}
	results := make([]any, b.n)
	for i := 0; i < b.n; i++ {
		row := logits[i*numLabels : (i+1)*numLabels]
		probs := softmax(row)
		type scored struct {
			Label string  `json:"label"`
			Score float64 `json:"score"`
			Index int     `json:"index"`
		}
		all := make([]scored, numLabels)
		for j, p := range probs {
			all[j] = scored{Label: s.labelName(j), Score: round(p), Index: j}
		}
		sort.SliceStable(all, func(a, c int) bool { return all[a].Score > all[c].Score })
		results[i] = map[string]any{
			"label":  all[0].Label,
			"score":  all[0].Score,
			"scores": all[:topK],
		}
	}
	if single {
		return results[0], nil
	}
	return map[string]any{"results": results}, nil
}

func (s *session) labelName(i int) string {
	if i < len(s.layout.Labels) && s.layout.Labels[i] != "" {
		return s.layout.Labels[i]
	}
	return fmt.Sprintf("LABEL_%d", i)
}

func softmax(x []float32) []float64 {
	maxV := float32(math.Inf(-1))
	for _, v := range x {
		if v > maxV {
			maxV = v
		}
	}
	out := make([]float64, len(x))
	sum := 0.0
	for i, v := range x {
		e := math.Exp(float64(v - maxV))
		out[i] = e
		sum += e
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

func round(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// embed returns pooled, optionally normalized sentence embeddings.
func (s *session) embed(req runtime.InferenceRequest) (any, error) {
	strs, single, err := texts(req.Input)
	if err != nil {
		return nil, err
	}
	b, err := s.tokenize(strs)
	if err != nil {
		return nil, err
	}
	outputs, err := s.runText(b)
	if err != nil {
		return nil, err
	}
	defer destroyAll(outputs)

	// Prefer a pre-pooled output if the model exports one.
	idx := 0
	for i, o := range s.outputs {
		if o.Name == "sentence_embedding" || o.Name == "pooler_output" {
			idx = i
			break
		}
	}
	data, shape, err := floatData(outputs[idx])
	if err != nil {
		return nil, err
	}
	var vectors [][]float32
	switch len(shape) {
	case 2: // [batch, hidden]
		hidden := int(shape[1])
		for i := 0; i < b.n; i++ {
			vectors = append(vectors, append([]float32(nil), data[i*hidden:(i+1)*hidden]...))
		}
	case 3: // [batch, seq, hidden]
		seq, hidden := int(shape[1]), int(shape[2])
		for i := 0; i < b.n; i++ {
			vec := make([]float32, hidden)
			switch s.cfg.pooling() {
			case "cls":
				copy(vec, data[i*seq*hidden:i*seq*hidden+hidden])
			case "none":
				// return the CLS row as well; token-level output is too large for JSON by default
				copy(vec, data[i*seq*hidden:i*seq*hidden+hidden])
			default:
				var count float32
				for t := 0; t < seq; t++ {
					if b.mask[i*b.seq+t] == 0 {
						continue
					}
					count++
					row := data[(i*seq+t)*hidden : (i*seq+t+1)*hidden]
					for h, v := range row {
						vec[h] += v
					}
				}
				if count > 0 {
					for h := range vec {
						vec[h] /= count
					}
				}
			}
			vectors = append(vectors, vec)
		}
	default:
		return nil, fmt.Errorf("unexpected embedding output shape %v", shape)
	}
	if s.cfg.normalize() {
		for _, v := range vectors {
			normalize(v)
		}
	}
	dims := 0
	if len(vectors) > 0 {
		dims = len(vectors[0])
	}
	if single {
		return map[string]any{"embedding": vectors[0], "dimensions": dims}, nil
	}
	return map[string]any{"embeddings": vectors, "dimensions": dims}, nil
}

func normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	n := float32(math.Sqrt(sum))
	if n == 0 {
		return
	}
	for i := range v {
		v[i] /= n
	}
}

func numberParam(params map[string]any, key string) (float64, bool) {
	if params == nil {
		return 0, false
	}
	switch v := params[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}
