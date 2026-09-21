# HTTP API

The API is served from the same address as the dashboard, normally
`http://127.0.0.1:9090`.

## Health and management

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/system/health` | Liveness check. |
| `GET` | `/api/system` | Host, database, and runtime information. |
| `GET` | `/api/runtimes` | Runtime availability. |
| `GET` | `/api/runtimes/{runtime}/logs?n=200` | Recent runtime logs (`llamacpp` or `onnx`). |
| `GET` | `/api/runtimes/{runtime}/logs/stream` | Live runtime logs over server-sent events. |
| `GET` | `/api/models` | List registered models and live status. |
| `POST` | `/api/models` | Register a model. |
| `GET` | `/api/models/{id}` | Get model metadata and status. |
| `PATCH` | `/api/models/{id}` | Update model metadata or config. |
| `DELETE` | `/api/models/{id}` | Unload and remove a model. |
| `POST` | `/api/models/{id}/load` | Load a registered model. |
| `POST` | `/api/models/{id}/unload` | Unload a model. |
| `POST` | `/api/models/{id}/restart` | Unload and load again. |
| `GET` | `/api/models/{id}/status` | Live status and runtime details. |
| `GET` | `/api/models/{id}/logs?n=200` | Recent model logs. |
| `GET` | `/api/models/{id}/logs/stream` | Server-sent log stream. |

Models can be addressed by registry name or ID. Live states are `stopped`, `starting`,
`running`, `failed`, and `stopping`.

### Live system telemetry

`GET /api/system` includes physical `memory.total_bytes` and
`memory.available_bytes`, host-wide `cpu.usage_percent`, and NVIDIA telemetry in
`gpu.devices`. GPU devices report identity and driver version, utilization percent,
used/total VRAM bytes, temperature in Celsius, and power in watts when supported.
These are host-wide measurements, not memory attributed only to registered models.

CPU utilization needs two samples; missing values mean unavailable, not zero.
CPU and physical memory collection support Windows and Linux. NVIDIA data requires
`nvidia-smi` on the server's PATH; queries are cached and time-bounded. A missing or
unsupported GPU is reported through `gpu.available` and `gpu.error` without failing
the rest of the system response. The dashboard refreshes telemetry every three seconds.

### Runtime log streams

The runtime stream sends a `snapshot` event containing a JSON object with `runtime`
and a `lines` array, followed by `line` events containing individual JSON log records. Replace
the displayed history on each snapshot, including after a reconnect. Heartbeat
comments keep idle connections alive. Logs are bounded, in-memory diagnostic
history, not a durable audit log. The dashboard shows streams by default and saves
the visibility preference in the browser; hiding or pausing logs closes the stream.

## Generic prediction

Use this endpoint for ONNX classification, embeddings, custom tensors, and runtime-agnostic
prediction:

```http
POST /v1/models/{model}/predict
Content-Type: application/json

{"input":"I loved this movie","params":{"top_k":2}}
```

The response contains `model`, `task`, `runtime`, `output`, and `timing`. The output is a
label and scores for classification, vectors for embeddings, or named tensors for custom
models.

## OpenAI-compatible endpoints

GGUF models expose:

```text
GET  /v1/models
POST /v1/chat/completions
POST /v1/completions
POST /v1/embeddings
```

Example:

```bash
curl http://127.0.0.1:9090/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model":"gemma",
    "messages":[{"role":"user","content":"Hello"}],
    "stream":true
  }'
```

The `model` value is the registry name. Any OpenAI-compatible client can use
`http://127.0.0.1:9090/v1` as its base URL.

## Authentication

When API keys are configured, send one on every `/api/*` and `/v1/*` request:

```bash
curl http://127.0.0.1:9090/api/models \
  -H 'Authorization: Bearer replace-with-a-long-secret'
```

Errors use an OpenAI-style envelope:

```json
{"error":{"message":"...","type":"...","code":"..."}}
```
