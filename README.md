# modelserver

A self-hosted, open-source server for **small local AI models**. Register GGUF and ONNX models
on your machine, load and serve them through OpenAI-compatible and generic prediction APIs, and
manage everything from a web dashboard or the CLI.

- **GGUF** models are served by [llama.cpp](https://github.com/ggml-org/llama.cpp) (`llama-server`)
  child processes, one per loaded model.
- **ONNX** models (classifiers, embedding models, arbitrary tensor models) run in-process with
  [ONNX Runtime](https://onnxruntime.ai).
- Single Go binary with the React dashboard embedded. SQLite registry. Works headless.

```
+---------------------------- modelserver (Go) ------------------------------+
|  CLI (cobra)   HTTP API (/api, /v1)   Web UI (embedded React, optional)     |
|                        \        |        /                                 |
|                          service layer  <-- SQLite registry (models table) |
|                          /            \                                    |
|          runtime.Runtime interface     logs.Store (per-model ring buffer)  |
|           /                    \                                           |
|  llamacpp runtime          onnx runtime                                    |
|  (spawns llama-server,     (ONNX Runtime sessions in memory,               |
|   health checks, proxy)     HF tokenizer.json, softmax / pooling)          |
+----------------------------------------------------------------------------+
```

The application layer only talks to `runtime.Runtime`; runtime-specific settings live in an
opaque JSON `config` blob on each model, so more runtimes can be added later without touching
the API, CLI or UI.

## Contents

- [Install](#install)
- [Runtimes](#runtimes)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [CLI](#cli)
- [HTTP API](#http-api)
- [Web dashboard](#web-dashboard)
- [Development](#development)
- [Scope](#scope)

## Install

### Build from source

Requires Go 1.25+ and (to rebuild the dashboard) Node 20+.

```bash
git clone https://github.com/donvito/open-models-server.git
cd open-models-server
(cd web && npm ci && npm run build)       # builds web/dist, embedded by go:embed
go build -o bin/modelserver ./cmd/modelserver
```

`go build` works without the frontend step too; the binary then runs API-only and `/` returns
a small JSON banner instead of the dashboard.

## Runtimes

Both runtimes are optional at startup. The System page and `modelserver system` show which
ones were detected; loading a model whose runtime is missing returns a clear error.

### llama.cpp (GGUF)

Install `llama-server` from a [llama.cpp release](https://github.com/ggml-org/llama.cpp/releases)
or build it yourself, then either put it on `PATH` or point modelserver at it:

```bash
modelserver serve --llama-binary /opt/llama.cpp/llama-server
# or MODELSERVER_LLAMA_CPP_BINARY=/opt/llama.cpp/llama-server
# or llamacpp.binary in modelserver.yaml
```

Each loaded GGUF model becomes a `llama-server` child process on an internal port
(default range `12000-12999`, loopback only). modelserver waits for `/health`, proxies
OpenAI-compatible requests to it, captures its stdout/stderr into the log buffer, and stops
it on unload or server shutdown. A crashed process is reported as `failed` (with the last
error line from its output); there is no automatic restart loop.

Per-model config (JSON):

| key              | meaning                                             |
|------------------|-----------------------------------------------------|
| `context_length` | `-c`                                                |
| `gpu_layers`     | `-ngl` (`-1` = all)                                 |
| `threads`        | `-t`                                                |
| `batch_size`     | `-b`                                                |
| `port`           | fixed internal port instead of one from the pool    |
| `extra_args`     | list of extra `llama-server` arguments              |

### ONNX Runtime

Download the ONNX Runtime shared library for your platform from the
[onnxruntime releases](https://github.com/microsoft/onnxruntime/releases) and either place
`libonnxruntime.so` / `libonnxruntime.dylib` / `onnxruntime.dll` next to the binary (or in
`/usr/local/lib`, `/opt/onnxruntime/lib`, `/opt/homebrew/lib`) or point at it:

```bash
modelserver serve --onnx-library /opt/onnxruntime/lib/libonnxruntime.so
# or MODELSERVER_ONNX_LIBRARY=... / onnx.library in modelserver.yaml
```

A model path may be a `.onnx` file or a directory (Hugging Face export layout):
`model.onnx` / `model_quantized.onnx`, `tokenizer.json`, and `config.json` (used for
`id2label` and `pad_token_id`). Supported tasks:

- `classification` – text in, `{label, score, scores[]}` out (softmax over logits).
- `embedding` – text in, vectors out (mean/CLS pooling, optional L2 normalisation).
- `custom` – raw named tensors in (`{"x": {"shape": [1,4], "data": [...]}}`), raw outputs out.

Per-model config (JSON): `model_file`, `tokenizer_file`, `max_length` (512), `labels`,
`pooling` (`mean` | `cls` | `none`), `normalize` (true), `threads`.

## Quick start

```bash
# 1. start the server (dashboard at http://localhost:9090)
modelserver serve --llama-binary /path/to/llama-server

# 2. register a GGUF model (runtime + task inferred from the .gguf extension)
modelserver models add ./models/gemma-2b-it.gguf --name gemma

# 3. load it
modelserver models load gemma

# 4. talk to it with any OpenAI client
curl http://localhost:9090/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemma","messages":[{"role":"user","content":"Hello!"}]}'

# 5. stop it
modelserver models unload gemma
```

ONNX:

```bash
modelserver models add ./models/sst2-onnx --runtime onnx --task classification --name sst2 --load
curl http://localhost:9090/v1/models/sst2/predict \
  -H 'Content-Type: application/json' -d '{"input":"I loved this movie"}'
# {"model":"sst2","task":"classification","runtime":"onnx",
#  "output":{"label":"POSITIVE","score":0.9998,"scores":[...]},"timing":{...}}
```

Headless (no dashboard, APIs only):

```bash
modelserver serve --headless
```

## Configuration

Precedence: **CLI flags > `MODELSERVER_*` env vars > `modelserver.yaml` > defaults**.
See [`modelserver.example.yaml`](modelserver.example.yaml) for every key.

| setting                  | default                | env                                          | flag              |
|--------------------------|------------------------|----------------------------------------------|-------------------|
| server.host              | `127.0.0.1`            | `MODELSERVER_HOST`                           | `--host`          |
| server.port              | `9090`                 | `MODELSERVER_PORT`                           | `--port`          |
| server.ui                | `true`                 | `MODELSERVER_UI`, `MODELSERVER_HEADLESS=1`   | `--headless/--ui` |
| database.path            | `./data/modelserver.db`| `MODELSERVER_DATABASE_PATH`                  | `--db`            |
| models.directory         | `./models`             | `MODELSERVER_MODELS_DIRECTORY`               | `--models-dir`    |
| llamacpp.binary          | `llama-server`         | `MODELSERVER_LLAMA_CPP_BINARY`               | `--llama-binary`  |
| llamacpp.port_range_*    | `12000`–`12999`        | `MODELSERVER_LLAMA_CPP_PORT_RANGE_START/END` |                   |
| llamacpp.startup_timeout_seconds | `600`          | `MODELSERVER_LLAMA_CPP_STARTUP_TIMEOUT_SECONDS` |                |
| onnx.library             | auto-detect            | `MODELSERVER_ONNX_LIBRARY`                   | `--onnx-library`  |
| auth.api_keys            | none (auth off)        | `MODELSERVER_API_KEYS` (comma separated)     | `--api-key`       |
| logs.lines_per_model     | `2000`                 | `MODELSERVER_LOGS_LINES_PER_MODEL`           |                   |

Config file location: `--config path`, else `MODELSERVER_CONFIG`, else `./modelserver.yaml`
if present.

### Authentication

Setting one or more API keys protects every `/api/*` and `/v1/*` route. Clients send
`Authorization: Bearer <key>` or `X-API-Key: <key>` (`?api_key=` is accepted for the SSE log
stream). `GET /api/system/health` and the dashboard shell stay public; the dashboard asks for
a key and stores it in the browser.

## CLI

The CLI uses the running server when one is reachable (so lifecycle commands work), and falls
back to the SQLite registry directly for list/add/remove when the server is down.

```
modelserver serve [--headless|--ui] [--host] [--port] ...
modelserver models list
modelserver models add <path> [--name] [--runtime llamacpp|onnx] [--task ...] [--config '{json}'] [--load]
modelserver models load|unload|restart|status|logs <name-or-id>
modelserver models remove <name-or-id>          # registry only; files are never deleted
modelserver system
```

Add `--json` to any command for machine-readable output.

## HTTP API

Errors use an OpenAI-style envelope: `{"error":{"message":"...","type":"...","code":"..."}}`.
Models can be referenced by registry `name` or `id` everywhere.

### Management

| method | path                          | description                                   |
|--------|-------------------------------|-----------------------------------------------|
| GET    | `/api/models`                 | list models with live status                  |
| POST   | `/api/models`                 | register `{name?, model_path, runtime?, task?, description?, config?}` |
| GET    | `/api/models/{id}`            | model + live status                           |
| PATCH  | `/api/models/{id}`            | update name/description/task/config           |
| DELETE | `/api/models/{id}`            | unload if needed and remove from registry     |
| POST   | `/api/models/{id}/load`       | load (blocks until running or failed)         |
| POST   | `/api/models/{id}/unload`     | stop                                          |
| POST   | `/api/models/{id}/restart`    | unload + load                                 |
| GET    | `/api/models/{id}/status`     | live status only                              |
| GET    | `/api/models/{id}/logs?n=200` | recent log lines                              |
| GET    | `/api/models/{id}/logs/stream`| Server-Sent Events (`event: log`)             |
| GET    | `/api/system`                 | host, memory, versions, runtime availability  |
| GET    | `/api/system/health`          | liveness (always public)                      |
| GET    | `/api/runtimes`               | runtime availability                          |

Statuses: `stopped`, `starting`, `running`, `failed`, `stopping`.

### OpenAI-compatible (llama.cpp models)

`GET /v1/models`, `POST /v1/chat/completions`, `POST /v1/completions`, `POST /v1/embeddings`.
The `model` field is the registry name; the request is proxied to that model's `llama-server`,
including `stream: true` SSE responses. Any OpenAI SDK works with
`base_url=http://localhost:9090/v1`.

### Generic prediction (any runtime)

```
POST /v1/models/{model}/predict
{"input": <string | string[] | object>, "params": {...}}
```

Returns `{"model","task","runtime","output","timing":{"latency_ms",...}}`. The `output` shape
depends on the task: classification → labels and scores, embedding → vectors, custom → named
tensors, chat/completion → the llama.cpp completion.

## Web dashboard

Served at `http://localhost:9090` unless `--headless`.

- **Models** – table of registered models with status, runtime, task, uptime and actions
  (load / unload / restart / edit / delete), plus an *Add model* dialog that infers runtime
  and task from the path.
- **Model detail** – overview, live streaming logs (filter / pause / clear), copy-paste API
  examples for that model, and settings.
- **Playground** – streaming chat for generative models; structured input/output view
  (score bars, embedding dimensions, raw JSON) for classification, embedding and custom models.
- **System** – OS, CPU, memory, listener, database path, and llama.cpp / ONNX Runtime
  availability with versions.

## Development

```bash
go test ./...                 # unit tests (fake runtimes; no llama-server/ORT needed)
go vet ./... && gofmt -l .
cd web && npm run dev         # Vite dev server on :5173 proxying /api and /v1 to :9090
cd web && npm run lint && npm run build
```

The ONNX integration test runs only when real assets are provided:

```bash
MODELSERVER_TEST_ONNX_LIBRARY=/path/libonnxruntime.so \
MODELSERVER_TEST_ONNX_CLASSIFIER=/path/to/sst2-onnx-dir \
go test ./internal/runtime/onnx/ -run TestRealClassification -v
```

Layout:

```
cmd/modelserver         entry point
internal/config         defaults, YAML, env, flag precedence
internal/models         runtime-agnostic model domain
internal/registry       SQLite persistence (migrations/ holds the schema)
internal/runtime        Runtime interface + registry
internal/runtime/llamacpp, internal/runtime/onnx
internal/process        child process + port allocation
internal/logs           per-model ring buffers + subscriptions (SSE)
internal/service        shared logic used by the API and the CLI
internal/api            HTTP handlers, auth, OpenAI proxy, SSE
internal/cli            cobra commands
web/                    React + Vite + Tailwind dashboard (embedded from web/dist)
```

## Scope

Deliberately out of scope for this first version: training, fine-tuning, model downloads,
GGUF conversion/quantization, Docker/Kubernetes orchestration, distributed inference,
multi-tenancy, billing, autoscaling, and runtimes other than llama.cpp and ONNX Runtime
(vLLM, MLX, Transformers). The runtime abstraction is the extension point for those later.

## License

MIT
