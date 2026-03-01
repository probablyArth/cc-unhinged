# cc-unhinged 🔥

> **Your Claude Code token burn? Now with sound effects.**

Ever watch your token count climb and feel that rising panic? This plugin **sonifies your existential dread** with escalating sound alerts as Claude devours your context budget.

Three thresholds. Three levels of *"fahhh"*. Pure chaos.

https://github.com/user-attachments/assets/your-demo-video.mp4

---

## 🎯 What it does

Plays sounds when your Claude Code session crosses token thresholds:

- **5K tokens** 😬 → *fahhh* (warning)
- **15K tokens** 😰 → *FAHHH* (high alert)
- **30K tokens** 💀 → ***FAHHHHHH*** (critical)

Perfect for:
- Audibly tracking when you asked Claude to "refactor the entire codebase real quick"
- Knowing when to stop before bankruptcy
- Scaring your coworkers

---

## ⚡ Quick Start

**1. Clone & build:**
```bash
git clone https://github.com/probablyArth/cc-unhinged.git
cd cc-unhinged
go build -o bin/cc-unhinged .
```

**2. Install in Claude Code:**
```
/plugins → Add local plugin → /path/to/cc-unhinged
```

**3. Burn tokens. Hear sounds. Regret everything.**

---

## 🎵 Soundpacks

Soundpacks are bundled sound sets in `sounds/<pack>/`. Each pack has three files: `warning.*`, `high.*`, `critical.*`.

**Bundled packs:**

| Pack | Vibe |
|------|------|
| `fahhh` | Escalating "fahhh" — the default existential dread |

**Select a pack** in `~/.cc-unhinged/config.json`:

```json
{
  "soundpack": "fahhh"
}
```

Or via environment variable: `CLAUDE_TOKEN_SOUNDPACK=fahhh`

**Add your own pack** — just create a new directory under `sounds/`:

```
sounds/
  fahhh/          ← bundled
    warning.wav
    high.wav
    critical.wav
  airhorn/        ← your custom pack
    warning.wav
    high.wav
    critical.wav
```

**Per-sound overrides** in `sounds.*` still take priority over the soundpack — mix and match as you please.

---

## 🎛️ Custom Configuration

Create `~/.cc-unhinged/config.json`:

```json
{
  "soundpack": "fahhh",
  "thresholds": {
    "warning": 5000,
    "high": 15000,
    "critical": 30000
  },
  "sounds": {
    "warning": "/path/to/override.wav",
    "high": "/path/to/override.wav",
    "critical": "/path/to/override.wav"
  },
  "player": "afplay",
  "debug": false
}
```

All fields are optional — omit any to use defaults.

Sound resolution order (highest wins):
1. **Environment variables** — per-session overrides
2. **Per-sound config** — `sounds.*` in config file
3. **Soundpack** — `soundpack` in config file
4. **OS system sounds** — Tink/Glass/Sosumi on macOS, freedesktop on Linux

---

## 🔧 How It Works

Two hooks track your token usage:

1. **`UserPromptSubmit`** (sync) — snapshots baseline before Claude starts
2. **`PostToolUse` / `SubagentStop` / `Stop`** (async) — checks if any thresholds were crossed, plays the highest sound

```
UserPromptSubmit → baseline = 4000 tokens
  ↓
PostToolUse      → 4200 tokens (no threshold crossed)
PostToolUse      → 5300 tokens → 🔊 plays warning sound
                   baseline = 5300
  ↓
Stop             → 5800 tokens (no NEW threshold crossed)
  ↓
Next prompt...
UserPromptSubmit → baseline = 5800 (fresh cycle)
```

**Key feature:** Alerts only fire for thresholds crossed **during the current prompt**. No spam, just pure panic at the right moments.

---

## 🧪 Test Without Burning Tokens

```bash
# Simulate crossing warning threshold
./bin/cc-unhinged --test 4000 5500

# Simulate crossing high threshold
./bin/cc-unhinged --test 6000 16000

# Simulate crossing ALL thresholds (plays critical only)
./bin/cc-unhinged --test 0 35000

# No thresholds crossed (silent)
./bin/cc-unhinged --test 4000 4800
```

---

## 🐛 Debug Mode

Turn on logging to see exactly what's happening:

```json
{
  "debug": true
}
```

Then watch the logs:
```bash
tail -f ~/.cc-unhinged/debug.log
```

---

## ⚙️ Advanced Configuration

### Environment Variables

Override thresholds and sounds per-session:

| Variable | Default | Description |
|----------|---------|-------------|
| `CLAUDE_TOKEN_WARNING` | `5000` | Warning threshold |
| `CLAUDE_TOKEN_HIGH` | `15000` | High threshold |
| `CLAUDE_TOKEN_CRITICAL` | `30000` | Critical threshold |
| `CLAUDE_TOKEN_SOUNDPACK` | none | Name of bundled soundpack |
| `CLAUDE_TOKEN_SOUND_WARNING` | system | Path to warning sound |
| `CLAUDE_TOKEN_SOUND_HIGH` | system | Path to high sound |
| `CLAUDE_TOKEN_SOUND_CRITICAL` | system | Path to critical sound |
| `CLAUDE_TOKEN_PLAYER` | `afplay`/`paplay` | Audio player binary |
| `CLAUDE_TOKEN_DEBUG` | off | Set to `1` for debug logs |

### Default System Sounds

| Level | macOS | Linux |
|-------|-------|-------|
| warning | Tink.aiff | freedesktop/message.oga |
| high | Glass.aiff | freedesktop/bell.oga |
| critical | Sosumi.aiff | freedesktop/alarm-clock-elapsed.oga |

### Manual Hook Installation

Don't want to use `/plugins`? Add to `~/.claude/settings.json`:

<details>
<summary>Click to expand hook config</summary>

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/cc-unhinged/bin/cc-unhinged",
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
            "command": "/path/to/cc-unhinged/bin/cc-unhinged",
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
            "command": "/path/to/cc-unhinged/bin/cc-unhinged",
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
            "command": "/path/to/cc-unhinged/bin/cc-unhinged",
            "timeout": 5,
            "async": true
          }
        ]
      }
    ]
  }
}
```

</details>

---

## 📊 Token Counting

Tokens are summed from `message.usage` in your session JSONL:

- ✅ `input_tokens` — non-cached input per turn
- ✅ `output_tokens` — generated tokens per turn
- ❌ Cache tokens (`cache_creation_input_tokens`, `cache_read_input_tokens`) — not counted (they're optimization, not new work)

---

## 🤝 Contributing

PRs welcome! Especially for:
- More cursed sound packs
- Windows support testing
- Better default sounds
- Meme-worthy threshold presets

---

## 📜 License

MIT — use it, break it, make it worse.

---

## ⭐ Star this repo if:
- You've ever said "just a quick refactor" and burned 50K tokens
- You want audible confirmation of your poor life choices
- Sound effects make everything better

---

**Made with ~~regret~~ love by [@probablyArth](https://github.com/probablyArth)**

*Now go forth and sonify your token burns.* 🔥
