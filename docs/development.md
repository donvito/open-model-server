# Development

## Build the UI and app together

Run `bash build.sh` on macOS/Linux or Git Bash, or `./build.cmd` on Windows
PowerShell/Command Prompt. PowerShell users can also run `./build.ps1`.
The scripts work from any current directory, install the locked frontend dependencies,
build the dashboard, and embed it in the Go executable. They stop on the first error.

Use `bash build.sh --skip-install` or `./build.cmd -SkipInstall` to reuse installed
frontend dependencies when the lockfile has not changed. The executable is
`bin/modelserver.exe` on Windows and `bin/modelserver` elsewhere. Stop the server
before rebuilding on Windows, then restart it with your usual runtime flags.

## Run the checks

```bash
go test ./...
go vet ./...
gofmt -l .
```

Frontend checks:

```bash
cd web
npm run lint
npm run build
```

The unit tests use fake runtimes and do not need `llama-server` or ONNX Runtime. The real
ONNX classification integration test runs only when model assets are supplied:

```bash
MODELSERVER_TEST_ONNX_LIBRARY=/path/libonnxruntime.so \
MODELSERVER_TEST_ONNX_CLASSIFIER=/path/to/sst2-onnx-dir \
go test ./internal/runtime/onnx/ -run TestRealClassification -v
```

On Windows Git Bash, set the variables with Unix-style paths or use PowerShell environment
variable syntax in a PowerShell terminal.

## Local frontend development

Run the Go server and Vite in separate terminals:

```bash
go run ./cmd/modelserver serve
cd web && npm run dev
```

Vite listens on port 5173 and proxies `/api` and `/v1` to port 9090. Run `npm run build`
before rebuilding the Go binary when you want the latest UI embedded in the executable.

## Repository layout

```text
cmd/modelserver          executable entry point
internal/config           defaults, YAML, environment, and flag precedence
internal/models           runtime-agnostic model domain
internal/registry         SQLite persistence and migrations
internal/runtime          runtime interface and registry
internal/runtime/llamacpp llama-server process management and proxying
internal/runtime/onnx     in-process ONNX Runtime sessions
internal/process          child processes and internal port allocation
internal/logs             per-model ring buffers and SSE subscriptions
internal/service          logic shared by the API and CLI
internal/api              HTTP handlers, auth, proxying, and SSE
internal/cli              Cobra commands
web                       React/Vite/Tailwind dashboard
```

The service layer is shared by the HTTP API and CLI. Runtime-specific settings are stored in
the model's JSON `config` field, which keeps lifecycle and API code independent of a runtime.
