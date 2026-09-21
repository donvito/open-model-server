# HTTP API

The API is served from the same address as the dashboard, normally
`http://127.0.0.1:9090`.

## Health and management

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/system/health` | Liveness check. |
| `GET` | `/api/system` | Host, database, and runtime information. |
| `GET` | `/api/runtimes` | Runtime availability. |
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
