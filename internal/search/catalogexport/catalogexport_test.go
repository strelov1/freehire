package catalogexport

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 13, 3, 0, 0, 0, time.UTC)

func batches(n, size int) Batch {
	return func(_ context.Context, after int64) ([][]byte, int64, error) {
		if after >= int64(n) {
			return nil, after, nil
		}
		var lines [][]byte
		for i := after + 1; i <= min(after+int64(size), int64(n)); i++ {
			lines = append(lines, []byte(fmt.Sprintf(`{"id":%d}`, i)))
		}
		return lines, after + int64(len(lines)), nil
	}
}

func TestKeys(t *testing.T) {
	if got := DataKey("2026-11-13"); got != "catalogue/2026-11-13/jobs.jsonl.gz" {
		t.Fatal(got)
	}
	if Day(time.Date(2026, 11, 13, 23, 30, 0, 0, time.FixedZone("x", -5*3600))) != "2026-11-14" {
		t.Fatal("day is not UTC")
	}
}

func TestWriteStreamsManifestAndGzip(t *testing.T) {
	b := newFake()
	m, err := Write(context.Background(), b, "2026-11-13", now, batches(25, 10))
	if err != nil {
		t.Fatal(err)
	}
	raw := b.objs[DataKey("2026-11-13")]
	sum := sha256.Sum256(raw)
	if m.Rows != 25 || m.Bytes != int64(len(raw)) || m.SHA256 != hex.EncodeToString(sum[:]) || m.SchemaVersion != SchemaVersion {
		t.Fatalf("manifest %+v does not describe the file (%d bytes)", m, len(raw))
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(zr)
	n := 0
	for sc.Scan() {
		n++
	}
	if n != 25 {
		t.Fatalf("file has %d lines, want 25", n)
	}
	got, err := List(context.Background(), b)
	if err != nil || len(got) != 1 || got[0] != m {
		t.Fatalf("List = %+v, %v", got, err)
	}
}

func TestFailedRunLeavesNoManifest(t *testing.T) {
	b := newFake()
	boom := errors.New("db gone")
	calls := 0
	_, err := Write(context.Background(), b, "2026-11-13", now, func(ctx context.Context, after int64) ([][]byte, int64, error) {
		if calls++; calls > 1 {
			return nil, 0, boom
		}
		return [][]byte{[]byte(`{}`)}, 1, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := b.objs[ManifestKey("2026-11-13")]; ok {
		t.Fatal("manifest written for a failed run")
	}
	if got, _ := List(context.Background(), b); len(got) != 0 {
		t.Fatalf("unfinished snapshot listed: %+v", got)
	}
}

func TestPruneKeepsNewestFinishedAndInProgress(t *testing.T) {
	b := newFake()
	ctx := context.Background()
	for _, d := range []string{"2026-11-08", "2026-11-09", "2026-11-10", "2026-11-11"} {
		if _, err := Write(ctx, b, d, now, batches(1, 1)); err != nil {
			t.Fatal(err)
		}
	}
	b.objs[DataKey("2026-11-07")] = []byte("x") // older leftover without manifest
	b.objs[DataKey("2026-11-12")] = []byte("x") // run in progress
	b.objs["resumes/1-abc"] = []byte("x")       // not ours

	if _, err := Prune(ctx, b, 2); err != nil {
		t.Fatal(err)
	}
	keys, _ := b.List(ctx, "")
	want := []string{DataKey("2026-11-10"), ManifestKey("2026-11-10"), DataKey("2026-11-11"), ManifestKey("2026-11-11"), DataKey("2026-11-12"), "resumes/1-abc"}
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Fatalf("left %v, want %v", keys, want)
	}
}

// flaky fails Delete of chosen keys.
type flaky struct {
	*fakeBucket
	failDel map[string]bool
}

func (f *flaky) Delete(ctx context.Context, key string) error {
	if f.failDel[key] {
		return errors.New("delete failed")
	}
	return f.fakeBucket.Delete(ctx, key)
}

func TestRerunLeavesPublishedDayUntouched(t *testing.T) {
	ctx := context.Background()
	b := newFake()
	first, err := Write(ctx, b, "2026-11-13", now, batches(3, 10))
	if err != nil {
		t.Fatal(err)
	}
	old := string(b.objs[DataKey("2026-11-13")])
	m, err := Write(ctx, b, "2026-11-13", now, batches(5, 10))
	if !errors.Is(err, ErrPublished) || m != first {
		t.Fatalf("rerun = %+v, %v; want the published manifest and ErrPublished", m, err)
	}
	if string(b.objs[DataKey("2026-11-13")]) != old {
		t.Fatal("published data replaced by a rerun")
	}
}

// ghostManifest lists a manifest that is gone by the time it is read, as when Prune runs
// between List and Get.
type ghostManifest struct{ *fakeBucket }

func (g ghostManifest) List(ctx context.Context, prefix string) ([]string, error) {
	keys, err := g.fakeBucket.List(ctx, prefix)
	return append(keys, ManifestKey("2026-11-01")), err
}

func TestListSkipsManifestDeletedMidList(t *testing.T) {
	ctx := context.Background()
	b := newFake()
	if _, err := Write(ctx, b, "2026-11-13", now, batches(1, 1)); err != nil {
		t.Fatal(err)
	}
	got, err := List(ctx, ghostManifest{b})
	if err != nil || len(got) != 1 || got[0].Date != "2026-11-13" {
		t.Fatalf("List = %+v, %v; want only 2026-11-13", got, err)
	}
}

func TestPruneKeepsDataWhenManifestDeleteFails(t *testing.T) {
	ctx := context.Background()
	b := &flaky{fakeBucket: newFake(), failDel: map[string]bool{}}
	for _, d := range []string{"2026-11-09", "2026-11-10", "2026-11-11"} {
		if _, err := Write(ctx, b, d, now, batches(1, 1)); err != nil {
			t.Fatal(err)
		}
	}
	b.failDel[ManifestKey("2026-11-09")] = true
	if _, err := Prune(ctx, b, 2); err == nil {
		t.Fatal("want delete error")
	}
	if _, ok := b.objs[DataKey("2026-11-09")]; !ok {
		t.Fatal("data deleted while its manifest is still listed")
	}
}
