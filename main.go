// Claude Token Alert — Escalating sound alerts for cumulative token usage.
//
// Stateless recalculation from session transcript. On each Stop event,
// reads the full JSONL transcript, sums input_tokens + output_tokens,
// and plays an escalating sound when thresholds are crossed.
//
// Config precedence: defaults ← ~/.cc-unhinged/config.json ← env vars
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type hookPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	HookEventName  string `json:"hook_event_name"`
}

type transcriptEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type assistantMessage struct {
	Usage *usageData `json:"usage"`
}

type usageData struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type configFile struct {
	Thresholds map[string]int    `json:"thresholds"`
	Sounds     map[string]string `json:"sounds"`
	Player     string            `json:"player"`
	Debug      bool              `json:"debug"`
}

type threshold struct {
	Level string
	Value int
}

// ---------------------------------------------------------------------------
// Config resolution: defaults ← config file ← env vars
// ---------------------------------------------------------------------------

var (
	isMac = runtime.GOOS == "darwin"
)

func defaultSounds() map[string]string {
	if isMac {
		return map[string]string{
			"warning":  "/System/Library/Sounds/Tink.aiff",
			"high":     "/System/Library/Sounds/Glass.aiff",
			"critical": "/System/Library/Sounds/Sosumi.aiff",
		}
	}
	return map[string]string{
		"warning":  "/usr/share/sounds/freedesktop/stereo/message.oga",
		"high":     "/usr/share/sounds/freedesktop/stereo/bell.oga",
		"critical": "/usr/share/sounds/freedesktop/stereo/alarm-clock-elapsed.oga",
	}
}

func defaultPlayer() string {
	if isMac {
		return "afplay"
	}
	return "paplay"
}

func loadConfigFile() configFile {
	home, err := os.UserHomeDir()
	if err != nil {
		return configFile{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".cc-unhinged", "config.json"))
	if err != nil {
		return configFile{}
	}
	var cfg configFile
	if json.Unmarshal(data, &cfg) != nil {
		return configFile{}
	}
	return cfg
}

func resolveInt(envKey string, cfgVal int, fallback int) int {
	if v, ok := os.LookupEnv(envKey); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	if cfgVal > 0 {
		return cfgVal
	}
	return fallback
}

func resolveString(envKey string, cfgVal string, fallback string) string {
	if v, ok := os.LookupEnv(envKey); ok && v != "" {
		return v
	}
	if cfgVal != "" {
		return cfgVal
	}
	return fallback
}

type config struct {
	Thresholds []threshold
	Sounds     map[string]string
	Player     string
	Debug      bool
}

// ---------------------------------------------------------------------------
// Debug logging — writes to ~/.cc-unhinged/debug.log when debug: true
// ---------------------------------------------------------------------------

var debugFile *os.File

func debugLog(format string, args ...any) {
	if debugFile == nil {
		return
	}
	ts := time.Now().Format("15:04:05.000")
	fmt.Fprintf(debugFile, "[%s] %s\n", ts, fmt.Sprintf(format, args...))
}

func initDebug(enabled bool) {
	if !enabled {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(home, ".cc-unhinged", "debug.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	debugFile = f
}

func loadConfig() config {
	cfg := loadConfigFile()
	if cfg.Thresholds == nil {
		cfg.Thresholds = map[string]int{}
	}
	if cfg.Sounds == nil {
		cfg.Sounds = map[string]string{}
	}
	ds := defaultSounds()

	debug := cfg.Debug
	if v, ok := os.LookupEnv("CLAUDE_TOKEN_DEBUG"); ok && v != "" && v != "0" && v != "false" {
		debug = true
	}

	return config{
		Thresholds: []threshold{
			{"warning", resolveInt("CLAUDE_TOKEN_WARNING", cfg.Thresholds["warning"], 5000)},
			{"high", resolveInt("CLAUDE_TOKEN_HIGH", cfg.Thresholds["high"], 15000)},
			{"critical", resolveInt("CLAUDE_TOKEN_CRITICAL", cfg.Thresholds["critical"], 30000)},
		},
		Sounds: map[string]string{
			"warning":  resolveString("CLAUDE_TOKEN_SOUND_WARNING", cfg.Sounds["warning"], ds["warning"]),
			"high":     resolveString("CLAUDE_TOKEN_SOUND_HIGH", cfg.Sounds["high"], ds["high"]),
			"critical": resolveString("CLAUDE_TOKEN_SOUND_CRITICAL", cfg.Sounds["critical"], ds["critical"]),
		},
		Player: resolveString("CLAUDE_TOKEN_PLAYER", cfg.Player, defaultPlayer()),
		Debug:  debug,
	}
}

// ---------------------------------------------------------------------------
// State — stores token baseline (snapshot before current prompt)
// ---------------------------------------------------------------------------

func statePath(sessionID string) string {
	return filepath.Join(os.TempDir(), "cc-unhinged-"+sessionID)
}

func readBaseline(sessionID string) int {
	data, err := os.ReadFile(statePath(sessionID))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return n
}

func writeBaseline(sessionID string, tokens int) {
	_ = os.WriteFile(statePath(sessionID), []byte(strconv.Itoa(tokens)), 0644)
}

// ---------------------------------------------------------------------------
// Token calculation — sum input_tokens + output_tokens from transcript
// ---------------------------------------------------------------------------

func calculateUsage(transcriptPath string) int {
	f, err := os.Open(transcriptPath)
	if err != nil {
		debugLog("ERROR opening transcript: %v", err)
		return 0
	}
	defer f.Close()

	total := 0
	turnCount := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10MB lines

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry transcriptEntry
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		if entry.Type != "assistant" || entry.Message == nil {
			continue
		}

		var msg assistantMessage
		if json.Unmarshal(entry.Message, &msg) != nil {
			continue
		}
		if msg.Usage == nil {
			continue
		}

		turnCount++
		turnTotal := msg.Usage.InputTokens + msg.Usage.OutputTokens
		total += turnTotal
		debugLog("  turn %d: input=%d output=%d turn_total=%d running_total=%d",
			turnCount, msg.Usage.InputTokens, msg.Usage.OutputTokens, turnTotal, total)
	}

	debugLog("transcript: %d assistant turns with usage, total=%d tokens", turnCount, total)
	return total
}

// ---------------------------------------------------------------------------
// Sound playback — fully detached, never blocks
// ---------------------------------------------------------------------------

func playSound(player string, path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	cmd := exec.Command(player, path)
	cmd.Stdout = nil
	cmd.Stderr = nil
	// SysProcAttr for detach is platform-specific; Popen-style Start() is enough
	// since we don't call Wait(), the process outlives us.
	_ = cmd.Start()
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	// --test <baseline> <total>: simulate a threshold check without a transcript
	// e.g. ./bin/token-alert --test 4000 5500
	if len(os.Args) >= 4 && os.Args[1] == "--test" {
		runTest(os.Args[2], os.Args[3])
		return
	}

	var payload hookPayload
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		return
	}
	if payload.SessionID == "" || payload.TranscriptPath == "" {
		return
	}

	cfg := loadConfig()
	initDebug(cfg.Debug)
	defer func() {
		if debugFile != nil {
			debugFile.Close()
		}
	}()

	debugLog("=== %s === session=%s", payload.HookEventName, payload.SessionID)

	// On UserPromptSubmit, snapshot current token count as baseline
	if payload.HookEventName == "UserPromptSubmit" {
		total := calculateUsage(payload.TranscriptPath)
		writeBaseline(payload.SessionID, total)
		debugLog("baseline snapshot: %d tokens", total)
		return
	}

	// Any other event — check which thresholds were crossed since baseline
	debugLog("transcript: %s", payload.TranscriptPath)
	debugLog("config: thresholds=%v player=%s", cfg.Thresholds, cfg.Player)
	debugLog("config: sounds=%v", cfg.Sounds)

	baseline := readBaseline(payload.SessionID)
	totalTokens := calculateUsage(payload.TranscriptPath)
	checkAndAlert(cfg, baseline, totalTokens, payload.SessionID)
}

func checkAndAlert(cfg config, baseline, totalTokens int, sessionID string) {
	debugLog("baseline=%d total_tokens=%d delta=%d", baseline, totalTokens, totalTokens-baseline)

	// Find thresholds crossed between baseline and current total
	var newAlerts []string
	for _, t := range cfg.Thresholds {
		crossedNow := totalTokens >= t.Value
		crossedBefore := baseline >= t.Value
		debugLog("  check %s: threshold=%d crossed_before=%v crossed_now=%v", t.Level, t.Value, crossedBefore, crossedNow)
		if crossedNow && !crossedBefore {
			newAlerts = append(newAlerts, t.Level)
		}
	}

	if len(newAlerts) == 0 {
		debugLog("no thresholds crossed this prompt, done")
		fmt.Fprintf(os.Stderr, "no thresholds crossed (baseline=%d total=%d)\n", baseline, totalTokens)
		return
	}

	// Play only the highest newly-crossed level (last in ordered list)
	highest := newAlerts[len(newAlerts)-1]
	debugLog("new_alerts=%v playing=%s sound=%s", newAlerts, highest, cfg.Sounds[highest])
	fmt.Fprintf(os.Stderr, "ALERT: %v (playing %s)\n", newAlerts, highest)
	if sound, ok := cfg.Sounds[highest]; ok {
		playSound(cfg.Player, sound)
	}

	// Advance baseline so subsequent events in this prompt don't re-trigger
	if sessionID != "" {
		writeBaseline(sessionID, totalTokens)
		debugLog("baseline advanced to %d, done", totalTokens)
	}
}

func runTest(baselineStr, totalStr string) {
	baseline, err := strconv.Atoi(baselineStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid baseline: %s\n", baselineStr)
		return
	}
	total, err := strconv.Atoi(totalStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid total: %s\n", totalStr)
		return
	}

	cfg := loadConfig()
	initDebug(cfg.Debug)
	defer func() {
		if debugFile != nil {
			debugFile.Close()
		}
	}()

	fmt.Fprintf(os.Stderr, "testing: baseline=%d total=%d\n", baseline, total)
	fmt.Fprintf(os.Stderr, "thresholds: warning=%d high=%d critical=%d\n",
		cfg.Thresholds[0].Value, cfg.Thresholds[1].Value, cfg.Thresholds[2].Value)
	checkAndAlert(cfg, baseline, total, "")
}
