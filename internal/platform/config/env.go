package config

import (
	"encoding/base64"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func parseNamedValues(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.TrimSpace(name) != "" && strings.TrimSpace(value) != "" {
			out[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseKeyRing(raw string) map[string][]byte {
	out := map[string][]byte{}
	for _, part := range strings.Split(raw, ",") {
		id, encoded, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok || strings.TrimSpace(id) == "" {
			continue
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			key = nil
		}
		// Preserve malformed named entries so NewKeyRing fails startup instead of
		// silently dropping an old decrypt-only key still referenced by grants.
		out[strings.TrimSpace(id)] = key
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// splitCSV parses a comma-separated env value into trimmed, non-empty entries (nil if none).
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// decodeKey base64-decodes an AES key, returning nil unless it is exactly 32 bytes.
func decodeKey(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil
	}
	return b
}

// decodeBase64 decodes a base64 env value, returning "" if unset or malformed. Used for
// OAUTH_APPLE_PRIVATE_KEY: a multi-line PEM does not survive a systemd EnvironmentFile.
func decodeBase64(s string) string {
	if s == "" {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// splitDomains parses a comma-separated COOKIE_DOMAIN into bare domains, trimming spaces and a
// leading dot. Empty entries are dropped.
func splitDomains(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimPrefix(strings.TrimSpace(p), ".")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// resolveTypstBin returns the absolute path of the typst binary, or "" if not on PATH, so a
// missing binary disables PDF rendering (501) instead of failing each render (500).
func resolveTypstBin(name string) string {
	if name == "" {
		return ""
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func envDuration(key string, fallback time.Duration) time.Duration {
	// An unparseable value also falls back.
	if d, err := time.ParseDuration(env(key, "")); err == nil {
		return d
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	// An unparseable value falls back, which for a spend ceiling means no ceiling.
	if f, err := strconv.ParseFloat(env(key, ""), 64); err == nil {
		return f
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	// Only an explicit "true" enables.
	if b, err := strconv.ParseBool(env(key, "")); err == nil {
		return b
	}
	return fallback
}
