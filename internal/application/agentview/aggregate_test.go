package agentview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestASearchThenADetailByOneViewerIsOneView(t *testing.T) {
	log := `{"ts":"2026-10-27T10:00:00Z","kind":"search","surface":"mcp","viewer":"a","job_ids":[1,2]}
{"ts":"2026-10-27T10:05:00Z","kind":"detail","surface":"mcp","viewer":"a","job_ids":[1]}
{"ts":"2026-10-27T11:00:00Z","kind":"detail","surface":"ojcp","viewer":"b","job_ids":[1]}
{"ts":"2026-10-28T00:00:01Z","kind":"detail","surface":"mcp","viewer":"a","job_ids":[1]}
`
	got, stats, err := Aggregate(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	d27 := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	d28 := d27.AddDate(0, 0, 1)
	if got[d27][1] != 2 || got[d27][2] != 1 || got[d28][1] != 1 {
		t.Errorf("counts = %v", got)
	}
	if stats.Lines != 4 || stats.Bad != 0 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestAnUnreadableLineIsCountedAndSkipped(t *testing.T) {
	_, stats, err := Aggregate(strings.NewReader("not json\n{\"ts\":\"2026-10-27T10:00:00Z\",\"viewer\":\"a\",\"job_ids\":[1]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Lines != 2 || stats.Bad != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestListFilesSkipsTodayAndStrangers(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"agent-views-2026-10-26.jsonl", "agent-views-2026-10-27.jsonl", "agent-views-2026-10-28.jsonl", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	files, err := ListFiles(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || filepath.Base(files[0].Path) != "agent-views-2026-10-26.jsonl" || filepath.Base(files[1].Path) != "agent-views-2026-10-27.jsonl" {
		t.Errorf("files = %+v", files)
	}

	// Just after midnight yesterday has not settled: the writer may still append to it.
	files, err = ListFiles(dir, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC).Add(SettleAfter-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0].Path) != "agent-views-2026-10-26.jsonl" {
		t.Errorf("before settling, files = %+v", files)
	}
}

func TestListFilesOfAMissingDirectoryIsEmpty(t *testing.T) {
	files, err := ListFiles(filepath.Join(t.TempDir(), "absent"), time.Now())
	if err != nil || len(files) != 0 {
		t.Errorf("files = %v, err = %v", files, err)
	}
}
