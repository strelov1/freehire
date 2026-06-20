// Package telegram ingests vacancies from public Telegram channels: crawling the
// web preview of each configured channel into the telegram_posts queue, and
// LLM-extracting structured vacancies from pending posts into the job catalogue.
package telegram

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Kind describes how a channel formats vacancies, steering the extraction prompt.
type Kind string

const (
	// KindAuthored is a curated/storytelling channel: one post holds 0..N vacancies.
	KindAuthored Kind = "authored"
	// KindBoard is a semi-structured job board channel: one post is one vacancy.
	KindBoard Kind = "board"
)

// ChannelEntry is one configured channel from sources/telegram.yml.
type ChannelEntry struct {
	Channel string `yaml:"channel"`
	Kind    Kind   `yaml:"kind"`
}

// Config is the parsed sources/telegram.yml: the set of channels to crawl.
type Config struct {
	Channels []ChannelEntry `yaml:"channels"`
}

// LoadChannels resolves the channel file from CHANNELS_FILE (default
// sources/telegram.yml), loads it, and validates it — the load+validate the crawl
// and extract workers both need. It fails fast so a misconfigured channel never
// starts a run.
func LoadChannels() (Config, error) {
	path := os.Getenv("CHANNELS_FILE")
	if path == "" {
		path = "sources/telegram.yml"
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Kinds maps each configured channel name to its kind, for the extraction prompt.
func (c Config) Kinds() map[string]Kind {
	m := make(map[string]Kind, len(c.Channels))
	for _, e := range c.Channels {
		m[e.Channel] = e.Kind
	}
	return m
}

// LoadConfig reads and parses a sources/telegram.yml file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("telegram: read config %s: %w", path, err)
	}
	return ParseConfig(data)
}

// ParseConfig parses sources/telegram.yml bytes into a Config.
func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("telegram: parse config: %w", err)
	}
	return cfg, nil
}

// Validate checks every entry has a channel, a known kind, and no duplicates, so
// the crawl command can fail fast instead of silently skipping or double-crawling.
func (c Config) Validate() error {
	seen := make(map[string]bool, len(c.Channels))
	for _, e := range c.Channels {
		if e.Channel == "" {
			return fmt.Errorf("telegram: entry with kind %q has empty channel", e.Kind)
		}
		if e.Kind != KindAuthored && e.Kind != KindBoard {
			return fmt.Errorf("telegram: channel %q has unknown kind %q", e.Channel, e.Kind)
		}
		if seen[e.Channel] {
			return fmt.Errorf("telegram: duplicate channel %q", e.Channel)
		}
		seen[e.Channel] = true
	}
	return nil
}
