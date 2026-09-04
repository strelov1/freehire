package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeChannels is a ChannelLister serving a fixed row set, or an error.
type fakeChannels struct {
	rows []db.ListActiveTelegramChannelsRow
	err  error
}

func (f fakeChannels) ListActiveTelegramChannels(context.Context) ([]db.ListActiveTelegramChannelsRow, error) {
	return f.rows, f.err
}

func rows(pairs ...string) []db.ListActiveTelegramChannelsRow {
	out := make([]db.ListActiveTelegramChannelsRow, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, db.ListActiveTelegramChannelsRow{Channel: pairs[i], Kind: pairs[i+1]})
	}
	return out
}

func TestLoadChannels(t *testing.T) {
	cfg, err := LoadChannels(context.Background(), fakeChannels{
		rows: rows("hrlunapark", "authored", "job_web3", "board"),
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Channels) != 2 {
		t.Fatalf("channels = %d, want 2", len(cfg.Channels))
	}
	if cfg.Channels[0].Channel != "hrlunapark" || cfg.Channels[0].Kind != KindAuthored {
		t.Errorf("first = %+v, want hrlunapark/authored", cfg.Channels[0])
	}
	if cfg.Channels[1].Kind != KindBoard {
		t.Errorf("second kind = %q, want board", cfg.Channels[1].Kind)
	}
}

// An empty catalog is an error, not an empty run: a crawl with nothing to crawl exits 0
// having done nothing, which is indistinguishable from a healthy quiet run.
func TestLoadChannelsRejectsAnEmptyCatalog(t *testing.T) {
	_, err := LoadChannels(context.Background(), fakeChannels{})
	if err == nil || !strings.Contains(err.Error(), "no active channels") {
		t.Fatalf("want a no-active-channels error, got %v", err)
	}
}

func TestLoadChannelsSurfacesTheQueryError(t *testing.T) {
	want := errors.New("connection refused")
	_, err := LoadChannels(context.Background(), fakeChannels{err: want})
	if err == nil || !strings.Contains(err.Error(), want.Error()) {
		t.Fatalf("want the query error surfaced, got %v", err)
	}
}

func TestValidateRejectsBadEntries(t *testing.T) {
	cases := []struct {
		name     string
		channels []ChannelEntry
		want     string // substring of the error
	}{
		{
			name:     "unknown kind",
			channels: []ChannelEntry{{Channel: "foo", Kind: "aggregator"}},
			want:     "aggregator",
		},
		{
			name:     "empty channel",
			channels: []ChannelEntry{{Channel: "", Kind: KindBoard}},
			want:     "empty channel",
		},
		{
			name:     "missing kind",
			channels: []ChannelEntry{{Channel: "foo"}},
			want:     "foo",
		},
		{
			name:     "duplicate channel",
			channels: []ChannelEntry{{Channel: "foo", Kind: KindBoard}, {Channel: "foo", Kind: KindAuthored}},
			want:     "duplicate",
		},
		{
			name:     "duplicate channel differing only by case",
			channels: []ChannelEntry{{Channel: "hrlunapark", Kind: KindAuthored}, {Channel: "HRLunapark", Kind: KindAuthored}},
			want:     "duplicate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Config{Channels: tc.channels}.Validate()
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %q", err, tc.want)
			}
		})
	}
}
