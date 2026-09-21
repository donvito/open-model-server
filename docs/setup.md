# Setup and startup

## Prerequisites

Building the server requires:

- Go 1.25 or newer.
- Node 20 or newer when rebuilding the dashboard.
- A C toolchain for the ONNX Runtime cgo integration. Use GCC or Clang on Linux/macOS and
  MinGW-w64 on Windows.

The llama.cpp and ONNX Runtime dependencies are optional at startup. The server can start
without either runtime, but loading a model that needs a missing runtime will fail.

## Build from source

From the repository root:

```bash
bash build.sh
```

On Windows PowerShell or Command Prompt, run `./build.cmd` instead (or
`./build.ps1` in PowerShell). Git Bash also supports `bash build.sh`.
The scripts install frontend dependencies with `npm ci`, build the UI, and then
compile the Go app. Output is `bin/modelserver.exe` on Windows and `bin/modelserver`
on macOS/Linux. Stop the running server before rebuilding on Windows.

For subsequent builds with dependencies already installed and the lockfile unchanged,
use `bash build.sh --skip-install` or `./build.cmd -SkipInstall`.

The frontend build creates `web/dist`, which is embedded into the Go binary. If the frontend
has not been built, the binary still serves the API but the dashboard is unavailable.

Start the built binary:

```bash
./bin/modelserver serve
```

On Windows Git Bash, the executable is normally `./bin/modelserver.exe`:

```bash
./bin/modelserver.exe serve
```

The default listener is `http://127.0.0.1:9090`. The same process serves the API and the
embedded dashboard. Press `Ctrl+C` to stop it.

For an API-only process:

```bash
./bin/modelserver.exe serve --headless
```

## Start with runtime paths

Pass both paths when using local Windows installations:

```bash
./bin/modelserver.exe serve \
  --llama-binary "C:/Users/you/llama.cpp/build/bin/llama-server.exe" \
  --onnx-library "C:/Users/you/onnxruntime/lib/onnxruntime.dll"
```

In Git Bash, `./` runs a local executable and `\` continues a command onto the next line.
Use forward slashes in Windows paths. PowerShell uses `./` as well, but uses a backtick for
line continuation.

On Windows, configure the real `llama-server.exe`. A Bash wrapper named `llama-server` may be
discoverable by the shell but cannot be resolved as a native executable by the Go process
launcher.

If you only need ONNX, omit `--llama-binary`. If you only need GGUF, omit `--onnx-library`.

## Check startup

The startup log reports each runtime:

```text
runtime available runtime=llamacpp ...
runtime available runtime=onnx ...
```

The system endpoint and CLI show the same information:

```bash
curl http://127.0.0.1:9090/api/system
./bin/modelserver.exe system
```

An `ONNX Runtime shared library ... not found` warning means the server was started without
the correct `--onnx-library` path, the DLL is not in an automatic search location, or the
runtime package is incomplete.

## Frontend development

The production-style server embeds the built UI. For Vite hot reload, use two terminals:

```bash
# terminal 1
go run ./cmd/modelserver serve

# terminal 2
cd web
npm run dev
```

Open <http://localhost:5173>. Vite proxies `/api` and `/v1` to the Go server on port 9090.

## Access from another machine

The default bind address is loopback. To listen on the network, configure `server.host` or
pass `--host 0.0.0.0`. Set an API key and restrict access with a firewall or private network
before exposing the server beyond localhost.
