// Package config loads runtime configuration from the environment (.env).
// Personal data never appears in logs — use Config.Redacted() for any logging.
package config

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Config holds the personal data and API credentials needed by agents.
// Populated from .env at startup; never committed to source control.
type Config struct {
	SubjectName  string // SUBJECT_NAME
	SubjectEmail string // SUBJECT_EMAIL
	SubjectState string // SUBJECT_STATE (two-letter, e.g. "OH")

	AnthropicKey string // ANTHROPIC_API_KEY
	GmailSender  string // GMAIL_SENDER
}

// Load reads config in priority order:
//  1. Real environment variables (already exported in your shell)
//  2. macOS Keychain  (service name "databrokergo-anthropic") — for ANTHROPIC_API_KEY only
//  3. .env file in the working directory
//
// Missing .env is not an error — the agent will refuse to run live
// (IsReady() will return an error explaining what's missing).
func Load() (*Config, error) {
	loadDotEnv(".env") // best-effort; silently ignored if absent

	// Resolve ANTHROPIC_API_KEY: env → Keychain → .env (already loaded above)
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		anthropicKey = keychainGet("databrokergo-anthropic")
	}

	return &Config{
		SubjectName:  os.Getenv("SUBJECT_NAME"),
		SubjectEmail: os.Getenv("SUBJECT_EMAIL"),
		SubjectState: os.Getenv("SUBJECT_STATE"),
		AnthropicKey: anthropicKey,
		GmailSender:  os.Getenv("GMAIL_SENDER"),
	}, nil
}

// keychainGet retrieves a secret from the macOS Keychain using the built-in
// `security` CLI.  Returns "" on any error (key not found, wrong OS, etc.).
// The value is never logged; it is only assigned to Config.AnthropicKey.
func keychainGet(service string) string {
	out, err := exec.Command(
		"security", "find-generic-password",
		"-s", service,
		"-w", // print password only, no metadata
	).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// IsReady returns a descriptive error if required fields are missing.
// Call this before dispatching any real agent — never run live without it.
func (c *Config) IsReady() error {
	missing := []string{}
	if c.SubjectName == "" {
		missing = append(missing, "SUBJECT_NAME")
	}
	if c.SubjectEmail == "" {
		missing = append(missing, "SUBJECT_EMAIL")
	}
	if c.SubjectState == "" {
		missing = append(missing, "SUBJECT_STATE")
	}
	if c.AnthropicKey == "" {
		missing = append(missing, "ANTHROPIC_API_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required .env fields: %s — copy .env.example to .env and fill in values",
			strings.Join(missing, ", "))
	}
	return nil
}

// Redacted returns a copy of the config with sensitive fields masked.
// Safe to pass to fmt.Printf or structured loggers.
func (c *Config) Redacted() Config {
	r := *c
	if r.AnthropicKey != "" {
		r.AnthropicKey = "sk-ant-***"
	}
	if len(r.SubjectEmail) > 3 {
		r.SubjectEmail = r.SubjectEmail[:3] + "***@***"
	}
	if r.SubjectName != "" {
		parts := strings.Fields(r.SubjectName)
		if len(parts) > 0 {
			r.SubjectName = parts[0][:1] + "*** " + parts[len(parts)-1][:1] + "***"
		}
	}
	return r
}

// loadDotEnv is a minimal KEY=VALUE parser that does not require an external
// dependency. It handles quoted values and ignores comments.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // silently skip missing .env
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		// Quoted value: extract content between the first matching quote pair,
		// ignoring anything after the closing quote (handles inline comments).
		if len(v) >= 1 && (v[0] == '"' || v[0] == '\'') {
			quote := v[0]
			end := strings.IndexByte(v[1:], quote)
			if end >= 0 {
				v = v[1 : end+1] // e.g. "CA"  # comment → CA
			} else {
				v = v[1:] // unclosed quote — take the rest
			}
		} else {
			// Unquoted value: strip inline comment (# preceded by whitespace).
			if idx := strings.Index(v, " #"); idx >= 0 {
				v = strings.TrimSpace(v[:idx])
			}
		}

		if os.Getenv(k) == "" { // don't override real env vars
			os.Setenv(k, v)
		}
	}
}
