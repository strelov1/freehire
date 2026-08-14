package telegram

import (
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
channels:
  - channel: hrlunapark
    kind: authored
  - channel: job_web3
    kind: board
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
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
	if err := cfg.Validate(); err != nil {
		t.Errorf("validate: %v, want nil", err)
	}
}

func TestValidateRejectsBadEntries(t *testing.T) {
	cases := []struct {
		name string
		yml  string
		want string // substring of the error
	}{
		{
			name: "unknown kind",
			yml: `
channels:
  - channel: foo
    kind: aggregator
`,
			want: "aggregator",
		},
		{
			name: "empty channel",
			yml: `
channels:
  - channel: ""
    kind: board
`,
			want: "empty channel",
		},
		{
			name: "missing kind",
			yml: `
channels:
  - channel: foo
`,
			want: "foo",
		},
		{
			name: "duplicate channel",
			yml: `
channels:
  - channel: foo
    kind: board
  - channel: foo
    kind: authored
`,
			want: "duplicate",
		},
		{
			name: "duplicate channel differing only by case",
			yml: `
channels:
  - channel: hrlunapark
    kind: authored
  - channel: HRLunapark
    kind: authored
`,
			want: "duplicate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(tc.yml))
			if err == nil {
				err = cfg.Validate()
			}
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %q", err, tc.want)
			}
		})
	}
}
