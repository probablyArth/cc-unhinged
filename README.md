# cc-unhinged

A Claude Code hook that plays escalating sound alerts when token usage crosses configurable thresholds during a prompt.

## How it works

Two hooks work together:

1. **`UserPromptSubmit`** (sync) — snapshots the current token count as a baseline before Claude starts processing
2. **`PostToolUse` / `SubagentStop` / `Stop`** (async) — recalculates total tokens, checks if any thresholds were crossed since the baseline, plays the highest newly-crossed alert sound

```
UserPromptSubmit → snapshot baseline (e.g. 4000 tokens)
  ↓
PostToolUse (Read)  → total=4200, no threshold crossed
PostToolUse (Bash)  → total=5300, crosses warning → plays sound
                      baseline advanced to 5300
  ↓
Stop → total=5800, no NEW threshold crossed (baseline=5300)
  ↓
next prompt...
UserPromptSubmit → snapshot baseline (5800)
```

Alerts only fire for thresholds crossed **during the current prompt**. The baseline advances after each alert to prevent duplicates within the same prompt, but resets on every new prompt.

## Install

### Option A: Claude Code plugin

```
/plugins → Add local plugin → /path/to/claude-cc-unhinged
```

### Option B: Direct hook in settings

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-cc-unhinged/bin/cc-unhinged",
            "timeout": 5
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-cc-unhinged/bin/cc-unhinged",
            "timeout": 5,
            "async": true
          }
        ]
      }
    ],
    "SubagentStop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-cc-unhinged/bin/cc-unhinged",
            "timeout": 5,
            "async": true
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-cc-unhinged/bin/cc-unhinged",
            "timeout": 5,
            "async": true
          }
        ]
      }
    ]
  }
}
```

## Build

```
go build -o bin/cc-unhinged .
```

## Test

Simulate threshold checks without burning tokens:

```bash
# ./bin/cc-unhinged --test <baseline> <total>

# crosses warning (plays warning sound)
./bin/cc-unhinged --test 4000 5500

# baseline past warning, crosses high (plays high sound)
./bin/cc-unhinged --test 6000 16000

# crosses nothing
./bin/cc-unhinged --test 4000 4800

# crosses all three (plays critical sound only)
./bin/cc-unhinged --test 0 35000
```

## Configuration

Config is resolved in layers (highest precedence wins):

1. **Environment variables** — per-session overrides
2. **`~/.cc-unhinged/config.json`** — persistent user config
3. **Built-in defaults**

### Config file

Create `~/.cc-unhinged/config.json`:

```json
{
  "thresholds": {
    "warning": 5000,
    "high": 15000,
    "critical": 30000
  },
  "sounds": {
    "warning": "/path/to/gentle-chime.wav",
    "high": "/path/to/alert.wav",
    "critical": "/path/to/airhorn.wav"
  },
  "player": "afplay",
  "debug": false
}
```

All fields are optional — omit any to use defaults.

### Debug logging

Set `"debug": true` in the config file (or `CLAUDE_TOKEN_DEBUG=1`) to write detailed logs:

```bash
tail -f ~/.cc-unhinged/debug.log
```

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `CLAUDE_TOKEN_WARNING` | `5000` | Warning threshold (tokens) |
| `CLAUDE_TOKEN_HIGH` | `15000` | High threshold (tokens) |
| `CLAUDE_TOKEN_CRITICAL` | `30000` | Critical threshold (tokens) |
| `CLAUDE_TOKEN_SOUND_WARNING` | system sound | Path to warning sound file |
| `CLAUDE_TOKEN_SOUND_HIGH` | system sound | Path to high sound file |
| `CLAUDE_TOKEN_SOUND_CRITICAL` | system sound | Path to critical sound file |
| `CLAUDE_TOKEN_PLAYER` | `afplay` / `paplay` | Audio player binary |
| `CLAUDE_TOKEN_DEBUG` | off | Set to `1` to enable debug logging |

### Default sounds

| Level | macOS | Linux |
|---|---|---|
| warning | Tink.aiff | freedesktop/message.oga |
| high | Glass.aiff | freedesktop/bell.oga |
| critical | Sosumi.aiff | freedesktop/alarm-clock-elapsed.oga |

## Token counting

Tokens are summed from `message.usage` on `type: "assistant"` entries in the session JSONL:

- `input_tokens` — non-cached input tokens per turn
- `output_tokens` — generated tokens per turn

Cache tokens (`cache_creation_input_tokens`, `cache_read_input_tokens`) are **not** counted — they represent context caching mechanics, not new work.
