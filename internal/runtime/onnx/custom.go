package onnx

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/donvito/modelserver/internal/models"
	"github.com/donvito/modelserver/internal/runtime"
)

// runCustom accepts raw tensors keyed by input name:
//
//	{"input": {"x": {"shape": [1, 4], "data": [1, 2, 3, 4]}}}
//
// A flat list is also accepted when the model has exactly one input, in which
// case the shape is [1, len(data)] unless the model declares a fixed shape.
func (s *session) runCustom(req runtime.InferenceRequest) (any, error) {
	spec, err := s.customInputs(req.Input)
	if err != nil {
		return nil, err
	}
	inputs := make([]ort.Value, len(s.inputs))
	defer destroyAll(inputs)
	for i, in := range s.inputs {
		t, ok := spec[in.Name]
		if !ok {
			return nil, fmt.Errorf("%w: missing input %q (model inputs: %v)", models.ErrValidation, in.Name, s.inputNames)
		}
		v, err := makeTensor(in, t)
		if err != nil {
			return nil, fmt.Errorf("%w: input %q: %v", models.ErrValidation, in.Name, err)
		}
		inputs[i] = v
	}
	outputs := make([]ort.Value, len(s.outputs))
	s.runMu.Lock()
	sess := s.sess
	if sess == nil {
		s.runMu.Unlock()
		return nil, runtime.ErrNotLoaded
	}
	err = sess.Run(inputs, outputs)
	s.runMu.Unlock()
	if err != nil {
		destroyAll(outputs)
		return nil, fmt.Errorf("onnx run: %w", err)
	}
	defer destroyAll(outputs)
	result := map[string]any{}
	for i, out := range s.outputs {
		result[out.Name] = valueToJSON(outputs[i])
	}
	return map[string]any{"outputs": result}, nil
}

type rawTensor struct {
	Shape []int64
	Data  []float64
}

func (s *session) customInputs(input any) (map[string]rawTensor, error) {
	out := map[string]rawTensor{}
	switch v := input.(type) {
	case map[string]any:
		for name, raw := range v {
			t, err := parseRawTensor(raw)
			if err != nil {
				return nil, fmt.Errorf("%w: input %q: %v", models.ErrValidation, name, err)
			}
			out[name] = t
		}
	case []any:
		if len(s.inputs) != 1 {
			return nil, fmt.Errorf("%w: model has %d inputs; send an object keyed by input name", models.ErrValidation, len(s.inputs))
		}
		t, err := parseRawTensor(map[string]any{"data": v})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
		}
		out[s.inputs[0].Name] = t
	default:
		return nil, fmt.Errorf("%w: input must be an object of tensors keyed by input name", models.ErrValidation)
	}
	return out, nil
}

func parseRawTensor(raw any) (rawTensor, error) {
	var t rawTensor
	switch v := raw.(type) {
	case map[string]any:
		if sh, ok := v["shape"].([]any); ok {
			for _, d := range sh {
				f, ok := d.(float64)
				if !ok {
					return t, fmt.Errorf("shape must be a list of integers")
				}
				t.Shape = append(t.Shape, int64(f))
			}
		}
		data, ok := v["data"]
		if !ok {
			return t, fmt.Errorf("missing 'data'")
		}
		if err := flatten(data, &t.Data, &t.Shape, 0); err != nil {
			return t, err
		}
	case []any:
		if err := flatten(v, &t.Data, &t.Shape, 0); err != nil {
			return t, err
		}
	case float64:
		t.Data = []float64{v}
		t.Shape = []int64{}
	default:
		return t, fmt.Errorf("tensor must be {shape, data} or a (nested) list of numbers")
	}
	return t, nil
}

// flatten appends numbers from a possibly nested list and infers the shape
// from nesting when no explicit shape was given.
func flatten(v any, data *[]float64, shape *[]int64, depth int) error {
	switch x := v.(type) {
	case float64:
		*data = append(*data, x)
		return nil
	case bool:
		if x {
			*data = append(*data, 1)
		} else {
			*data = append(*data, 0)
		}
		return nil
	case []any:
		if len(*shape) <= depth {
			*shape = append(*shape, int64(len(x)))
		}
		for _, e := range x {
			if err := flatten(e, data, shape, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("data must contain only numbers")
}

func makeTensor(info ort.InputOutputInfo, t rawTensor) (ort.Value, error) {
	shape := t.Shape
	if shape == nil {
		shape = ort.NewShape(1, int64(len(t.Data)))
	}
	if int64(len(t.Data)) != ort.Shape(shape).FlattenedSize() {
		return nil, fmt.Errorf("shape %v does not match %d values", shape, len(t.Data))
	}
	sh := ort.Shape(shape)
	switch info.DataType {
	case ort.TensorElementDataTypeFloat:
		return ort.NewTensor(sh, convert(t.Data, func(f float64) float32 { return float32(f) }))
	case ort.TensorElementDataTypeDouble:
		return ort.NewTensor(sh, append([]float64(nil), t.Data...))
	case ort.TensorElementDataTypeInt64:
		return ort.NewTensor(sh, convert(t.Data, func(f float64) int64 { return int64(f) }))
	case ort.TensorElementDataTypeInt32:
		return ort.NewTensor(sh, convert(t.Data, func(f float64) int32 { return int32(f) }))
	case ort.TensorElementDataTypeUint8:
		return ort.NewTensor(sh, convert(t.Data, func(f float64) uint8 { return uint8(f) }))
	case ort.TensorElementDataTypeBool:
		return ort.NewTensor(sh, convert(t.Data, func(f float64) bool { return f != 0 }))
	}
	return nil, fmt.Errorf("unsupported input type %s", info.DataType)
}

func convert[T any](in []float64, f func(float64) T) []T {
	out := make([]T, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

func valueToJSON(v ort.Value) any {
	switch t := v.(type) {
	case *ort.Tensor[float32]:
		return map[string]any{"shape": t.GetShape(), "data": t.GetData()}
	case *ort.Tensor[float64]:
		return map[string]any{"shape": t.GetShape(), "data": t.GetData()}
	case *ort.Tensor[int64]:
		return map[string]any{"shape": t.GetShape(), "data": t.GetData()}
	case *ort.Tensor[int32]:
		return map[string]any{"shape": t.GetShape(), "data": t.GetData()}
	case *ort.Tensor[uint8]:
		return map[string]any{"shape": t.GetShape(), "data": t.GetData()}
	case *ort.Tensor[bool]:
		return map[string]any{"shape": t.GetShape(), "data": t.GetData()}
	case *ort.StringTensor:
		strs, _ := t.GetContents()
		return map[string]any{"shape": t.GetShape(), "data": strs}
	}
	return map[string]any{"shape": v.GetShape(), "type": v.GetONNXType().String(), "data": nil}
}
