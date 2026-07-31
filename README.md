# moa-go

A Go port of the Mixture-of-Agents (MoA) CLI runner.

`moa-go` runs a panel of reference language models in parallel and synthesizes their outputs through an aggregator model. It is a standalone, native-HTTP implementation with no Rust, `rig-core`, or `pi.dev` dependencies.

## Quick start

### 1. Build

Requires Go 1.22 or later.

```bash
git clone https://github.com/jamespakele/moa-go.git
cd moa-go
go build -o moa ./cmd/moa
```

### 2. Create a config

```bash
./moa init
```

This writes `moa.toml` in the current directory with a default Ollama-based panel.

### 3. Run

```bash
./moa run "What is 2+2?"
```

Output is written to `output/moa-<timestamp>-<hash>.md`.

## Installation

Copy or symlink the built `moa` binary to a directory on your `$PATH`:

```bash
cp moa ~/.local/bin/
# or
ln -s $(pwd)/moa ~/.local/bin/moa
```

Then run from anywhere:

```bash
moa init
moa run "Your prompt here"
```

## Configuration

Edit `moa.toml` to change models, providers, synthesis parameters, and optional web-search verification.

```toml
[aggregator]
provider = "ollama"
model = "glm-5.2:cloud"
temperature = 0.4
max_tokens = 4096

[[reference]]
provider = "ollama"
model = "nemotron-3-super:cloud"
label = "nemotron"

[[reference]]
provider = "ollama"
model = "qwen3.5:397b-cloud"
label = "qwen35"

[[reference]]
provider = "ollama"
model = "deepseek-v4-flash:cloud"
label = "deepseek-v4-flash"

[backend]
backend = "native"
timeout_secs = 120
reference_thinking = "low"

[synthesis]
temperature = 0.4
max_tokens = 4096
output_dir = "./output"
max_rounds = 0
bmad_compatible = false
bmad_config_path = "_bmad/bmm/config.yaml"

[search]
provider = "tavily"
api_key = ""
max_queries = 5

[verifier]
enabled = false
provider = ""
model = ""
temperature = 0.2
max_tokens = 4096
```

### Providers

- **ollama** — local or remote Ollama. Remote hosts require `OLLAMA_API_KEY` or `api_key` in `[backend]`.
- **openrouter** — requires `OPENROUTER_API_KEY` or `api_key` in `[backend]`.

### API keys

Keys are resolved in this order:

1. Explicit `api_key` in `[backend]` (if non-empty).
2. Provider-specific environment variable (`OLLAMA_API_KEY`, `OPENROUTER_API_KEY`, `TAVILY_API_KEY`).
3. For Ollama on `localhost` / `127.0.0.1` / `::1`, no key is required.

### CLI flags

```bash
moa run "prompt"                          # positional prompt
printf 'prompt' | moa run -               # stdin prompt
moa run "prompt" --dry-run                # print what would run
moa run "prompt" --skill ./persona.md     # load skill/persona
moa run "prompt" --file ./context.md     # attach context file
moa run "prompt" -c /path/to/moa.toml    # custom config
moa run "prompt" --system-prompt "..."    # override system prompt
moa run "prompt" --append-system-prompt "..."
```

## Commands

| Command | Description |
|---|---|
| `moa init` | Create a default `moa.toml` |
| `moa run` | Execute the MoA pipeline |
| `moa config` | Display current configuration |
| `moa add-agent provider/model` | Add a reference agent |
| `moa remove-agent provider/model` | Remove a reference agent |

## Development

```bash
go test ./...
go build -o moa ./cmd/moa
```

## License

MIT. See [LICENSE](./LICENSE).
