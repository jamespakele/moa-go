---
name: moa-go
description: >
  Operate the local moa-go CLI binary for iterative Mixture-of-Agents runs.
  Builds the binary if needed, bootstraps moa.toml, inspects and edits
  configuration, runs prompts, attaches skills/personas and context files,
  reads generated outputs, and loops — adjusting settings and re-running
  until the result is good enough or the iteration budget is exhausted.
argument-hint: "[init | config | run <prompt> | add-agent <provider/model> | remove-agent <provider/model> | output <path>]"
---

# moa-go

A skill for driving the local `moa` binary hands-on.

Use it to:

- Bootstrap a fresh project with `moa init`
- Run MoA prompts against `moa.toml`
- Inspect and tweak configuration between runs
- Attach skills/personas and context files
- Iterate: run → read output → adjust → run again

---

## When to invoke

- The user asks to run the `moa` binary
- The user wants to change a MoA setting and re-run
- The user wants to add or remove a reference agent
- The user wants to inspect a generated output file
- The user says `/moa-go`, "run moa", "call moa", "iterate with moa", etc.

---

## Inputs

The first argument selects the operation. Remaining arguments depend on it.

| Operation | Arguments | Example |
|---|---|---|
| `init` | `[--force]` | `/moa-go init` |
| `config` | none | `/moa-go config` |
| `run` | `<prompt>` plus optional flags | `/moa-go run "What is 2+2?"` |
| `add-agent` | `<provider/model>` `[--label name]` | `/moa-go add-agent openrouter/anthropic/claude-3.5-sonnet --label claude` |
| `remove-agent` | `<provider/model>` | `/moa-go remove-agent openrouter/anthropic/claude-3.5-sonnet` |
| `output` | `<path>` | `/moa-go output output/moa-20260731-073121-b1a0c3c4.md` |

Optional flags for `run` (position-insensitive):

- `--dry-run` — print what would run, do not call LLMs
- `--skill <path>` — attach a skill/persona file (repeatable; first sets aggregator skill)
- `--file <path>` — attach a context file (repeatable)
- `--system-prompt "..."` — override the system prompt
- `--append-system-prompt "..."` — append to the system prompt
- `--config <path>` — use a custom `moa.toml`
- `--max-iterations N` — when auto-iterating, stop after N runs (default 3)

---

## Step 1 — Locate or build the `moa` binary

Try, in order:

1. `moa` on `$PATH` (run `which moa`)
2. `./moa` in the current working directory
3. `moa` in the `ai-moa-go` project root at `/srv/data/1-projects/ai-projects/ai-moa-go/moa`
4. Build it from source in `ai-moa-go`: `go build -o moa ./cmd/moa`

Stop at the first one that works. Remember the resolved binary path for the rest of the run. If none work, report the failure and stop.

---

## Step 2 — Resolve the working directory and config

- The working directory is the directory where the command runs.
- The default config file is `moa.toml` in that directory.
- If `--config <path>` was provided, use that path instead.
- For any operation other than `init`, if the config file does not exist, run `moa init` first (or warn and stop if `--config` was explicit and missing).

---

## Step 3 — Execute the requested operation

### `init`

Run the binary with `init`. If `moa.toml` already exists and the user did not pass `--force`, stop and tell the user to pass `--force` or delete the file.

```bash
./moa init
# or with force:
./moa init --force
```

### `config`

Run the binary with `config` to print the current configuration.

```bash
./moa config
```

Also read `moa.toml` directly so you can quote exact values if you need to edit them later.

### `run`

1. Parse the prompt from the remaining arguments. If the prompt is `-`, read from stdin.
2. Collect optional `--skill`, `--file`, `--system-prompt`, `--append-system-prompt`, `--config`, `--dry-run`, and `--max-iterations` flags.
3. Build the command line:

```bash
./moa run "<prompt>" \
  [--dry-run] \
  [--skill <path>]... \
  [--file <path>]... \
  [--system-prompt "..."] \
  [--append-system-prompt "..."] \
  [--config <path>]
```

4. Execute it.
5. Capture stderr and stdout. The binary prints a preview and the output file path on stderr.
6. If the run failed, print the error output and stop.
7. If it succeeded, read the generated output file and summarize what was produced.

### `add-agent`

Run:

```bash
./moa add-agent <provider/model> [--label <label>]
```

Then display the updated configuration with `moa config` so the user can verify.

### `remove-agent`

Run:

```bash
./moa remove-agent <provider/model>
```

Then display the updated configuration.

### `output`

Read the requested output file and summarize it for the user. If the path is relative, resolve it from the working directory.

---

## Step 4 — Iterate when asked (or when the result is unsatisfactory)

If the user explicitly asks to iterate, or if the output clearly does not satisfy the prompt, loop:

1. **Inspect** the latest output file.
2. **Diagnose** what is wrong. Common fixes:
   - Output too short / shallow → raise `synthesis.max_tokens`
   - Aggregator missing nuance → change `aggregator.model`
   - Reference panel unbalanced → `add-agent` or `remove-agent`
   - Need reasoning traces → set `backend.reference_thinking` to `"low"` or `"medium"`
   - Need verification → enable `[verifier]` and set `TAVILY_API_KEY`
   - Need multi-round deliberation → raise `synthesis.max_rounds`
   - Need a persona → add `--skill <file>` or set `aggregator.skill`
3. **Edit** `moa.toml` surgically. Read it first, then change exactly the values you intend.
4. **Re-run** the same prompt with the same attachments.
5. **Compare** the new output to the previous one.
6. **Repeat** up to `--max-iterations` times (default 3).

After each iteration, report:

```
Iteration {N}/{max}
Config changes: <what changed>
Output file: <path>
Status: <better / worse / same / meets intent>
```

If the result still does not meet intent after the budget is exhausted, stop and explain what you tried and what remains wrong. Do not loop forever.

---

## Editing `moa.toml`

Use the `read` tool to load the config, then the `edit` tool to change specific lines. Common edits:

- Change aggregator model:
  ```toml
  [aggregator]
  model = "new-model:tag"
  ```
- Change synthesis parameters:
  ```toml
  [synthesis]
  max_tokens = 8192
  max_rounds = 2
  ```
- Change reference thinking:
  ```toml
  [backend]
  reference_thinking = "medium"
  ```
- Add an OpenRouter block by uncommenting the example at the bottom of the file (or use `moa add-agent`).
- Enable verifier:
  ```toml
  [verifier]
  enabled = true
  provider = "openrouter"
  model = "anthropic/claude-3.5-sonnet"
  ```

Always re-run `moa config` after editing to confirm the binary sees the new values.

---

## Output format

For `run` and `iterate` operations, report:

```
*moa-go* — run
Binary: <path>
Config: <moa.toml path>
Command: <exact command line>
Status: <success | failed>
Output file: <path>
Preview: <first 500 chars of result, or the full stderr preview>
```

For `config`:

```
*moa-go* — config
<moa config output>
```

For `add-agent` / `remove-agent`:

```
*moa-go* — agent change
<command>
Updated reference count: <N>
```

For `output`:

```
*moa-go* — output summary
File: <path>
Length: <chars>
Summary: <what the output contains>
```

---

## Self-check

Before finishing any operation:

- [ ] Binary was located or built successfully
- [ ] The correct `moa.toml` was used
- [ ] The command line matches the user's intent and all provided flags
- [ ] For `run`/`iterate`, the output file was produced and is readable
- [ ] For edits, `moa config` confirms the change took effect
- [ ] No infinite loops: iteration count is bounded by `--max-iterations`

---

## Failure modes

- **Binary not found and build fails** → report the build error and stop.
- **No prompt provided for `run`** → ask the user for the prompt.
- **`moa.toml` missing for `run`/`config`** → run `moa init` first, then proceed.
- **Run fails (e.g., model not reachable)** → print the error, suggest a config fix, and ask whether to retry.
- **Output file missing after success message** → warn and inspect `output/` or the configured `synthesis.output_dir`.

---

## Notes

- API keys are never written into `moa.toml` unless the user explicitly asks. Prefer environment variables (`OLLAMA_API_KEY`, `OPENROUTER_API_KEY`, `TAVILY_API_KEY`).
- The binary writes outputs under `synthesis.output_dir` (default `./output`).
- `--dry-run` is safe: it loads config and prints the plan but does not call any LLM.
- When iterating, make one change at a time so you can attribute the effect.
