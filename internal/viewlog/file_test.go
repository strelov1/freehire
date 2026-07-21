package viewlog

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRotatedFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeGz := func(name, content string) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write([]byte(content))
		zw.Close()
		if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("access.log", "LIVE")
	write("access.log.1", "ONE")
	writeGz("access.log.2.gz", "TWO")
	write("error.log", "unrelated")

	files, err := RotatedFiles(dir, "access.log")
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, f := range files {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("Open(%s): %v", f.Path, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		got[filepath.Base(f.Path)] = string(b)
	}

	if len(got) != 2 {
		t.Fatalf("got %d rotated files (%v), want 2", len(got), got)
	}
	if got["access.log.1"] != "ONE" {
		t.Errorf("access.log.1 = %q, want ONE", got["access.log.1"])
	}
	if got["access.log.2.gz"] != "TWO" {
		t.Errorf("access.log.2.gz = %q (gzip not decompressed?), want TWO", got["access.log.2.gz"])
	}
	if _, ok := got["access.log"]; ok {
		t.Errorf("live access.log must be skipped")
	}
	if _, ok := got["error.log"]; ok {
		t.Errorf("unrelated error.log must be skipped")
	}
}

func TestRotatedFilesMissingDir(t *testing.T) {
	files, err := RotatedFiles(filepath.Join(t.TempDir(), "nope"), "access.log")
	if err != nil {
		t.Fatalf("missing dir should be a clean empty result, got err %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files for missing dir, want 0", len(files))
	}
}
