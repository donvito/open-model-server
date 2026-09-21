# Configuration

## Precedence

Settings are resolved in this order:

```text
CLI flags > MODELSERVER_* environment variables > modelserver.yaml > defaults
```

The config file is selected by `--config`, then `MODELSERVER_CONFIG`, then
`./modelserver.yaml` when that file exists. Start from [modelserver.example.yaml](../modelserver.example.yaml).

## Settings

| Setting | Default | Environment variable | CLI flag |
| --- | --- | --- | --- |
| `server.host` | `127.0.0.1` | `MODELSERVER_HOST` | `--host` |
| `server.port` | `9090` | `MODELSERVER_PORT` | `--port` |
| `server.ui` | `true` | `MODELSERVER_UI` | `--ui` |
| `database.path` | `./data/modelserver.db` | `MODELSERVER_DATABASE_PATH` | `--db` |
| `models.directory` | `./models` | `MODELSERVER_MODELS_DIRECTORY` | `--models-dir` |
| `llamacpp.binary` | `llama-server` | `MODELSERVER_LLAMA_CPP_BINARY` | `--llama-binary` |
| `llamacpp.port_range_start` | `12000` | `MODELSERVER_LLAMA_CPP_PORT_RANGE_START` | — |
| `llamacpp.port_range_end` | `12999` | `MODELSERVER_LLAMA_CPP_PORT_RANGE_END` | — |
| `llamacpp.startup_timeout_seconds` | `600` | `MODELSERVER_LLAMA_CPP_STARTUP_TIMEOUT_SECONDS` | — |
| `onnx.library` | auto-detect | `MODELSERVER_ONNX_LIBRARY` | `--onnx-library` |
| `auth.api_keys` | empty | `MODELSERVER_API_KEYS` | `--api-key` |
| `logs.lines_per_model` | `2000` | `MODELSERVER_LOGS_LINES_PER_MODEL` | — |

`MODELSERVER_HEADLESS=1` also disables the UI. `--headless` takes precedence over the UI
setting.

## Example config

```yaml
server:
  host: 127.0.0.1
  port: 9090
  ui: true

database:
  path: ./data/modelserver.db

models:
  directory: ./models

llamacpp:
  binary: C:/Users/you/llama.cpp/build/bin/llama-server.exe
  port_range_start: 12000
  port_range_end: 12999
  startup_timeout_seconds: 600

onnx:
  library: C:/Users/you/onnxruntime/lib/onnxruntime.dll

auth:
  api_keys:
    - replace-with-a-long-secret

logs:
  lines_per_model: 2000
```

## Network access and authentication

The default host is loopback, so only the local machine can connect. To serve another
machine, bind to a network interface:

```bash
./bin/modelserver.exe serve --host 0.0.0.0 --api-key replace-with-a-long-secret
```

Protect the port with a firewall or private network. API keys protect `/api/*` and `/v1/*`;
the health endpoint and dashboard shell remain public. Clients can send a key as either:

```text
Authorization: Bearer <key>
X-API-Key: <key>
```

The SSE log endpoint also accepts `?api_key=<key>` because browsers cannot set headers for an
`EventSource` URL without additional application code.
