package sources

import (
	"reflect"
	"testing"
)

func TestParseShard(t *testing.T) {
	cases := []struct {
		in   string
		i, n int
		ok   bool
	}{
		{"1/6", 1, 6, true},
		{"6/6", 6, 6, true},
		{"3/4", 3, 4, true},
		{"1/1", 1, 1, true},
		{"", 0, 0, false},
		{"0/6", 0, 0, false},   // i must be >= 1
		{"7/6", 0, 0, false},   // i must be <= n
		{"1/0", 0, 0, false},   // n must be >= 1
		{"abc", 0, 0, false},   // not "i/n"
		{"1", 0, 0, false},     // missing n
		{"1/2/3", 0, 0, false}, // too many parts
		{"-1/3", 0, 0, false},  // negative
		{"a/3", 0, 0, false},   // non-numeric i
		{"1/b", 0, 0, false},   // non-numeric n
	}
	for _, c := range cases {
		i, n, err := ParseShard(c.in)
		if c.ok {
			if err != nil || i != c.i || n != c.n {
				t.Errorf("ParseShard(%q) = (%d,%d,%v), want (%d,%d,nil)", c.in, i, n, err, c.i, c.n)
			}
		} else if err == nil {
			t.Errorf("ParseShard(%q) = (%d,%d,nil), want an error", c.in, i, n)
		}
	}
}

func companiesOf(c Config) []string {
	out := make([]string, len(c.Sources))
	for i, e := range c.Sources {
		out[i] = e.Company
	}
	return out
}

func mkConfig(names ...string) Config {
	e := make([]CompanyEntry, len(names))
	for i, nm := range names {
		e[i] = CompanyEntry{Company: nm, Provider: "workday", Board: nm}
	}
	return Config{Provider: "workday", Sources: e}
}

func TestConfigShard_RoundRobinPartition(t *testing.T) {
	cfg := mkConfig("a", "b", "c", "d", "e", "f", "g")

	// Round-robin: shard i (1-based) of n takes entries at indices where idx%n == i-1.
	want := map[int][]string{1: {"a", "d", "g"}, 2: {"b", "e"}, 3: {"c", "f"}}
	for i := 1; i <= 3; i++ {
		if got := companiesOf(cfg.Shard(i, 3)); !reflect.DeepEqual(got, want[i]) {
			t.Errorf("Shard(%d,3) = %v, want %v", i, got, want[i])
		}
	}
}

func TestConfigShard_PartitionsCompletelyNoOverlap(t *testing.T) {
	cfg := mkConfig("a", "b", "c", "d", "e", "f", "g", "h", "i", "j")
	seen := map[string]int{}
	for i := 1; i <= 4; i++ {
		for _, c := range companiesOf(cfg.Shard(i, 4)) {
			seen[c]++
		}
	}
	if len(seen) != 10 {
		t.Fatalf("union covered %d entries, want all 10", len(seen))
	}
	for c, n := range seen {
		if n != 1 {
			t.Errorf("entry %q appeared in %d shards, want exactly 1", c, n)
		}
	}
}

// All boards of one company MUST land in the same shard: the stale-job sweep scopes
// closes by company_slug, so splitting a company across shards would let one shard
// close the still-live boards another shard owns.
func TestConfigShard_KeepsACompanysBoardsTogether(t *testing.T) {
	// x and y each own two boards, interleaved so a naive index split would scatter them.
	cfg := Config{Provider: "workday", Sources: []CompanyEntry{
		{Company: "x", Provider: "workday", Board: "x1"},
		{Company: "y", Provider: "workday", Board: "y1"},
		{Company: "x", Provider: "workday", Board: "x2"},
		{Company: "z", Provider: "workday", Board: "z1"},
		{Company: "y", Provider: "workday", Board: "y2"},
	}}
	const n = 2
	companyShard := map[string]int{}
	for i := 1; i <= n; i++ {
		for _, e := range cfg.Shard(i, n).Sources {
			if prev, ok := companyShard[e.Company]; ok && prev != i {
				t.Errorf("company %q appears in shards %d and %d — a company must stay in one shard", e.Company, prev, i)
			}
			companyShard[e.Company] = i
		}
	}
	if len(companyShard) != 3 {
		t.Errorf("covered %d companies, want 3 (x,y,z)", len(companyShard))
	}
}

func TestConfigShard_SingleShardIsFullConfig(t *testing.T) {
	cfg := mkConfig("a", "b", "c")
	if got := companiesOf(cfg.Shard(1, 1)); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("Shard(1,1) = %v, want the full config", got)
	}
	if cfg.Shard(1, 3).Provider != "workday" {
		t.Error("Shard must preserve the file's default provider")
	}
}
