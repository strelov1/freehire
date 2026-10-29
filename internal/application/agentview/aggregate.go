package agentview

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DayFile is one day's log file.
type DayFile struct {
	Path string
	Day  time.Time
}

// SettleAfter is how long after its UTC day ends a day file is first read. The writer
// stops appending to a day lateWriteCutoff after it ends, so a file read past this is
// final, and the signature the rollup claims for it never changes afterwards.
const SettleAfter = 30 * time.Minute

// ListFiles returns the day files under dir whose day ended at least SettleAfter before
// now, oldest first.
// Today's file is still being written, so it is never offered. A missing dir is an empty
// answer: where the log is not configured there is nothing to roll up.
func ListFiles(dir string, now time.Time) ([]DayFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// A day is finished when it ended SettleAfter ago, i.e. it lies before this cut's day.
	cut := now.UTC().Add(-SettleAfter).Truncate(24 * time.Hour)
	var out []DayFile
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, filePrefix) || !strings.HasSuffix(n, fileSuffix) {
			continue
		}
		day, err := time.Parse(dayLayout, strings.TrimSuffix(strings.TrimPrefix(n, filePrefix), fileSuffix))
		if err != nil || !day.Before(cut) {
			continue
		}
		out = append(out, DayFile{Path: filepath.Join(dir, n), Day: day})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out, nil
}

// Stats reports what Aggregate read: every non-empty line, and how many of them it could
// not use.
type Stats struct{ Lines, Bad int }

// Aggregate counts unique viewers per (UTC day, job). A viewer seeing a job in a search
// and then reading it is ONE view: kind and surface are not in the dedup key.
func Aggregate(r io.Reader) (map[time.Time]map[int64]int, Stats, error) {
	counts := map[time.Time]map[int64]int{}
	seen := map[string]struct{}{}
	var stats Stats
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		stats.Lines++
		var e Event
		if err := json.Unmarshal(line, &e); err != nil || e.TS.IsZero() || e.Viewer == "" {
			stats.Bad++
			continue
		}
		day := e.TS.UTC().Truncate(24 * time.Hour)
		for _, id := range e.JobIDs {
			key := e.Viewer + "\x00" + strconv.FormatInt(id, 10) + "\x00" + day.Format(dayLayout)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			if counts[day] == nil {
				counts[day] = map[int64]int{}
			}
			counts[day][id]++
		}
	}
	return counts, stats, sc.Err()
}
