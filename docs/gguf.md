# GGUF and llama.cpp

## Install llama.cpp

Download a `llama-server` release from the [llama.cpp releases](https://github.com/ggml-org/llama.cpp/releases)
page or build it yourself. Modelserver must be able to launch the server executable locally.

Use an explicit path when it is not a native executable on `PATH`:

```bash
./bin/modelserver.exe serve \
  --llama-binary "C:/Users/you/llama.cpp/build/bin/llama-server.exe"
```

On Unix:

```bash
./bin/modelserver serve --llama-binary /opt/llama.cpp/llama-server
```

A bare `llama-server` name is looked up on `PATH` and then next to the modelserver binary.
On Windows, a shell script wrapper without `.exe` is not a reliable target; use the actual
`llama-server.exe` path.

## Register and load

The runtime and task are inferred from a `.gguf` path. A name makes later commands easier:

```bash
./bin/modelserver.exe models add \
  "C:/models/gemma-2b-it.gguf" \
  --name gemma \
  --load
```

Or register first and load later:

```bash
./bin/modelserver.exe models add "C:/models/gemma-2b-it.gguf" --name gemma
./bin/modelserver.exe models load gemma
```

Test an OpenAI-compatible model:

```bash
curl http://127.0.0.1:9090/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemma","messages":[{"role":"user","content":"Hello"}]}'
```

Unload, reload, or inspect a model without stopping the main server:

```bash
./bin/modelserver.exe models unload gemma
./bin/modelserver.exe models load gemma
./bin/modelserver.exe models status gemma
./bin/modelserver.exe models logs gemma
```

## Multiple GGUF models

Multiple models can be loaded at the same time. Each loaded GGUF model gets its own
`llama-server` child process and an internal loopback port from the default `12000-12999`
range. The modelserver API remains on port 9090.

Each child process loads its own model weights. Loading a second large model can therefore
exhaust VRAM or RAM. If the second model enters `failed`, inspect its status and logs before
changing the port range. A model configuration with a fixed `port` must use a different port
for every loaded model.

## Per-model settings

Pass JSON with `--config` or edit the model in the dashboard:

| Key | llama.cpp option | Purpose |
| --- | --- | --- |
| `context_length` | `-c` | Context size. |
| `gpu_layers` | `-ngl` | GPU layers; `-1` means all layers. |
| `threads` | `-t` | CPU thread count. |
| `batch_size` | `-b` | Batch size. |
| `port` | — | Fixed internal port instead of automatic allocation. |
| `extra_args` | — | Additional arguments passed to `llama-server`. |

Example:

```bash
./bin/modelserver.exe models add "C:/models/gemma.gguf" \
  --name gemma \
  --config '{"context_length":4096,"gpu_layers":-1,"threads":8}' \
  --load
```

## Vision models

For a vision-capable GGUF model, pass the matching projector in `extra_args`:

```bash
./bin/modelserver.exe models add "C:/models/vision-model.gguf" \
  --name vision \
  --config '{"extra_args":["--mmproj","C:/models/mmproj.gguf"]}' \
  --load
```

When the running llama.cpp build reports `modalities.vision`, the playground displays an
image attachment button. PNG, JPEG, WebP, and GIF files up to 5 MB each are accepted. The
full request, including conversation history and base64 images, is limited to 32 MB. Older
llama.cpp builds that do not report this capability keep the text-only composer.

## Reasoning models

Some instruct models stream their analysis in `reasoning_content` before the final answer.
If `max_tokens` is too low, the model can spend the whole budget reasoning and leave no visible
answer. The playground disables this by default; use its **Thinking** checkbox to enable it.
When enabled, the playground shows the streamed reasoning in a collapsible **Thinking** panel;
that display-only text is not sent back as part of the next conversation request.

For a direct API request, pass the chat-template setting explicitly:

```json
{"chat_template_kwargs":{"enable_thinking":false}}
```

## Common failures

- **`llama-server binary ... not found`**: pass the real executable with `--llama-binary`.
- **Missing CUDA/Vulkan DLL or `0xC0000135` on Windows**: install the provider DLLs that came
  with the llama.cpp build and keep them beside the executable, or use a CPU build.
- **Second model fails while the first runs**: check available VRAM/RAM, the model logs, and
  whether both models were configured with the same fixed internal port.
