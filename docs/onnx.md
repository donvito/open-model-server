# ONNX Runtime

## Install the native runtime

Modelserver loads the native ONNX Runtime shared library directly. Installing the Python
`onnx` package alone is not enough.

Download the CPU Windows x64 package, such as `onnxruntime-win-x64-<version>.zip`, from the
[ONNX Runtime releases](https://github.com/microsoft/onnxruntime/releases) page. Extract it
without separating its DLLs. The current modelserver integration creates default CPU
sessions; a CUDA package alone does not enable GPU execution in this integration.

Point modelserver at the extracted DLL:

```bash
./bin/modelserver.exe serve \
  --onnx-library "C:/Users/you/onnxruntime-win-x64-1.30.0/lib/onnxruntime.dll"
```

Keep any provider and dependency DLLs from the same release beside `onnxruntime.dll`. The
server adds that directory to the Windows DLL search path when it loads the library.

On Linux and macOS, use the matching `libonnxruntime.so` or `libonnxruntime.dylib` path. The
library can also be set with `MODELSERVER_ONNX_LIBRARY` or `onnx.library` in the YAML config.

## Model layout

For text classification or embeddings, a directory normally contains:

```text
my-model/
  model.onnx
  tokenizer.json
  config.json
```

`model_quantized.onnx`, `model_int8.onnx`, and `model_fp16.onnx` are also recognized. Hugging
Face Optimum layouts with `my-model/onnx/model.onnx` are supported when the tokenizer and
config are in the parent directory.

A single `.onnx` file can be registered directly. Text tasks still need a compatible
`tokenizer.json` beside it or an explicit `config.tokenizer_file`.

## Supported tasks

- **`classification`**: accepts text and returns labels and probabilities. The ONNX model
  must have a sequence-classification head whose output is `[batch, labels]`.
- **`embedding`**: accepts text and returns vectors. Rank-2 pooled output or rank-3 token
  output is supported; rank-3 output is pooled by mean by default.
- **`custom`**: accepts raw named tensors and returns raw named tensors. It does not tokenize
  text for you.

## Working classifier example

The small DistilBERT SST-2 model below is already exported to ONNX and includes its model,
tokenizer, and `NEGATIVE`/`POSITIVE` labels:

[`optimum/distilbert-base-uncased-finetuned-sst-2-english`](https://huggingface.co/optimum/distilbert-base-uncased-finetuned-sst-2-english)

Download it with the Hugging Face CLI:

```bash
python -m pip install -U huggingface_hub
hf download optimum/distilbert-base-uncased-finetuned-sst-2-english \
  --local-dir "C:/Users/you/models/distilbert-sst2"
```

Register and load it:

```bash
./bin/modelserver.exe models add \
  "C:/Users/you/models/distilbert-sst2" \
  --name sentiment \
  --runtime onnx \
  --task classification \
  --load
```

Predict:

```bash
curl http://127.0.0.1:9090/v1/models/sentiment/predict \
  -H 'Content-Type: application/json' \
  -d '{"input":"I loved this movie"}'
```

## ModernBERT and base models

`answerdotai/ModernBERT-base` is a masked-language model intended for fill-mask predictions,
not a sequence classifier. It can be loaded as a `custom` ONNX model for raw tensor testing,
but modelserver does not currently expose a `fill-mask` task. Use a model fine-tuned for
sequence classification with `--task classification`, or export a compatible encoder for
`--task embedding`.

## Per-model ONNX settings

The optional JSON config supports:

| Key | Purpose |
| --- | --- |
| `model_file` | Relative or absolute `.onnx` path. |
| `tokenizer_file` | Tokenizer path when it is not `tokenizer.json` beside the model. |
| `max_length` | Text truncation length; default `512`. |
| `labels` | Override labels from `config.json`. |
| `pooling` | Embedding pooling: `mean`, `cls`, or `none`. |
| `normalize` | L2-normalize embeddings; default `true`. |
| `threads` | ONNX Runtime intra-op thread count. |

Example:

```bash
./bin/modelserver.exe models add "C:/models/encoder" \
  --name encoder \
  --runtime onnx \
  --task embedding \
  --config '{"pooling":"mean","normalize":true,"max_length":256}' \
  --load
```

For `custom`, send tensors keyed by the ONNX input names. For example:

```json
{
  "input": {
    "x": {"shape": [1, 4], "data": [1, 2, 3, 4]}
  }
}
```

Use the model status endpoint to inspect its input and output names before constructing a
custom request.
