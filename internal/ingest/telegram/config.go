// Package telegram ingests vacancies from public Telegram channels: crawling the
// web preview of each configured channel into the telegram_posts queue, and
// LLM-extracting structured vacancies from pending posts into the job catalogue.
package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Kind describes how a channel formats vacancies, steering the extraction prompt.
type Kind string

const (
	// KindAuthored is a curated/storytelling channel: one post holds 0..N vacancies.
	KindAuthored Kind = "authored"
	// KindBoard is a semi-structured job board channel: one post is one vacancy.
	KindBoard Kind = "board"
)

// ChannelEntry is one configured channel from the telegram_channels table.
type ChannelEntry struct {
	Channel string
	Kind    Kind
}

// Config is the set of channels to crawl.
type Config struct {
	Channels []ChannelEntry
}

// ChannelLister is the read the crawl and extract workers need from the catalog, named
// narrowly so the dependency says so: *db.Queries satisfies it, and nothing else about
// the query layer is reachable from here.
type ChannelLister interface {
	ListActiveTelegramChannels(ctx context.Context) ([]db.ListActiveTelegramChannelsRow, error)
}

// LoadChannels reads the active channels and validates them — the load+validate the
// crawl and extract workers both need. It fails fast so a misconfigured channel never
// starts a run.
//
// An empty catalog is an error, not an empty run: a crawl with nothing to crawl exits 0
// having done nothing, which is indistinguishable from a healthy quiet run.
func LoadChannels(ctx context.Context, q ChannelLister) (Config, error) {
	rows, err := q.ListActiveTelegramChannels(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("telegram: list channels: %w", err)
	}
	if len(rows) == 0 {
		return Config{}, fmt.Errorf("telegram: no active channels configured")
	}
	cfg := Config{Channels: make([]ChannelEntry, len(rows))}
	for i, row := range rows {
		cfg.Channels[i] = ChannelEntry{Channel: row.Channel, Kind: Kind(row.Kind)}
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

// Validate checks every entry has a channel, a known kind, and no duplicates, so
// the crawl command can fail fast instead of silently skipping or double-crawling.
// The duplicate check is case-insensitive, matching t.me username matching
// elsewhere in the package (see preview.go's postID): "hrlunapark" and
// "HRLunapark" name the same channel and would otherwise both pass as distinct
// entries, crawling and extracting the same posts twice under different
// external_ids.
//
// The table's CHECK constraint and case-folded unique index enforce the same three
// rules. This is not redundant: it is what turns a schema violation nobody reads into a
// named error at the point of use, and it is the only check a hand-built Config (a test,
// a future admin path) passes through at all.
func (c Config) Validate() error {
	seen := make(map[string]bool, len(c.Channels))
	for _, e := range c.Channels {
		if e.Channel == "" {
			return fmt.Errorf("telegram: entry with kind %q has empty channel", e.Kind)
		}
		if e.Kind != KindAuthored && e.Kind != KindBoard {
			return fmt.Errorf("telegram: channel %q has unknown kind %q", e.Channel, e.Kind)
		}
		key := strings.ToLower(e.Channel)
		if seen[key] {
			return fmt.Errorf("telegram: duplicate channel %q", e.Channel)
		}
		seen[key] = true
	}
	return nil
}
