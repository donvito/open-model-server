# modelserver

`modelserver` is a self-hosted server for local GGUF and ONNX models. One Go binary
provides the HTTP API, an embedded web dashboard, a SQLite model registry, and model
lifecycle management.

- GGUF models run through local [`llama-server`](https://github.com/ggml-org/llama.cpp)
  child processes.
- ONNX models run in-process through [ONNX Runtime](https://onnxruntime.ai).
- Multiple models can be registered and loaded at the same time, subject to available
  RAM/VRAM and per-model port configuration.

## Quick start

The build requires Go 1.25+. Node 20+ is required when rebuilding the dashboard. Build the
dashboard before building the Go binary so the UI is embedded:

```bash
git clone https://github.com/donvito/open-models-server.git
cd open-models-server
bash build.sh
```

Start the API and dashboard together:

```bash
./bin/modelserver serve
```

On Windows PowerShell or Command Prompt, build with `./build.cmd` instead. Git Bash
can use `bash build.sh` as shown above. Both produce `bin/modelserver.exe` on Windows;
start it with `./bin/modelserver.exe serve`. Stop a running server before rebuilding.
The dashboard and API are available at
<http://127.0.0.1:9090>. Use `--headless` for an API-only server.

If a runtime is installed outside the normal search paths, pass its executable or shared
library explicitly:

```bash
./bin/modelserver serve \
  --llama-binary "C:/path/to/llama-server.exe" \
  --onnx-library "C:/path/to/onnxruntime.dll"
```

The two runtime options are independent; omit either one when that runtime is not needed.

## Load a model

Register and load a GGUF model:

```bash
./bin/modelserver models add ./models/gemma-2b-it.gguf --name gemma --load
```

Register and load an ONNX text classifier:

```bash
./bin/modelserver models add ./models/sst2-onnx \
  --name sentiment --runtime onnx --task classification --load
```

Use the dashboard, or manage models from the CLI:

```bash
./bin/modelserver models list
./bin/modelserver models status gemma
./bin/modelserver models unload gemma
```

GGUF models expose OpenAI-compatible chat and completion endpoints. ONNX models use the
generic prediction endpoint:

```bash
curl http://127.0.0.1:9090/v1/models/sentiment/predict \
  -H 'Content-Type: application/json' \
  -d '{"input":"I loved this movie"}'
```

## Configure

Configuration precedence is:

```text
CLI flags > MODELSERVER_* environment variables > modelserver.yaml > defaults
```

Copy [modelserver.example.yaml](modelserver.example.yaml) when you need persistent settings.
The main settings are the server address and port, database path, model directory, runtime
paths, internal llama.cpp port range, and API keys.

## Guides

- [Setup and startup](docs/setup.md) — build, run, Windows paths, embedded UI, and development mode.
- [GGUF and llama.cpp](docs/gguf.md) — install `llama-server`, load GGUF models, vision models,
  per-model settings, and troubleshooting.
- [ONNX Runtime](docs/onnx.md) — install the native runtime, use ONNX model layouts, supported
  tasks, and a working classifier example.
- [Configuration](docs/configuration.md) — configuration file, environment variables, ports,
  authentication, and network access.
- [HTTP API](docs/api.md) — management, prediction, and OpenAI-compatible endpoints.
- [Development](docs/development.md) — tests, linting, frontend development, and repository layout.

## License

MIT
