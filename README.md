# DevEdu Code

A terminal coding assistant — like Claude Code — powered by your **DevEdu** account.
It authenticates with your personal DevEdu API key and talks to an AI model through
DevEdu's Amazon Bedrock proxy (`POST /api/v1/chat`).

## Build

```bash
cd code
go build -o devedu .
```

## Configure

Get your API key from your DevEdu instance: **Account → API key**
(`<your-devedu>/account/api_key`). Then:

```bash
export DEVEDU_API_KEY=dvedu_xxxxxxxx…
export DEVEDU_API_URL=https://app.devedu.io   # your DevEdu instance (optional)
```

Both can also be passed as flags (`--api-key`, `--url`), which override the env.

## Use

```bash
# interactive session — a full-screen TUI
./devedu

# one-shot
./devedu "write a bubble sort in Go"
```

The interactive session is a full-screen **TUI** (Bubble Tea): a scrollable
transcript, a bordered input box, and a "thinking" spinner.
- type a request and press **Enter** to send (paste keeps multi-line)
- **↑/↓**, **PgUp/PgDn** scroll the transcript
- **/exit** or **Ctrl-C** quits

(When stdin isn't a terminal — piped input / CI — it falls back to a simple
line-based loop instead of the TUI.)

## Layout

```
code/
  main.go                  # CLI entry: flags, one-shot vs interactive
  internal/config/         # resolve API key + base URL (flags > env > default)
  internal/client/         # DevEdu API client (POST /api/v1/chat, Bearer auth)
  internal/repl/           # line-based loop (non-TTY fallback)
  internal/tui/            # full-screen TUI (Bubble Tea) for interactive use
```

## Roadmap toward "real" agentic coding

The DevEdu API is currently **single-turn** (`{prompt} → {response}`), so today the
CLI keeps a running transcript and resends it for context. To make it behave like
Claude Code — reading/writing files and running commands — the next steps are:

1. **Server side:** extend the DevEdu API to expose Bedrock's *Messages* API with
   conversation history and **tool use** (so the model can request file reads,
   edits, and shell commands), and ideally streaming.
2. **CLI side:** add a tool layer (`read_file`, `write_file`, `run_command`) and an
   agent loop that executes the model's tool calls and feeds results back, with
   user confirmation for writes/commands.

The package boundaries here (`client` / `repl`) are set up so this can grow without
a rewrite.
