package viewlog

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LogFile is a rotated access-log file. The worker keys its cursor on a signature
// of the file's decompressed content (see the worker), which is stable across both
// logrotate's numeric-suffix rename (.1 -> .2) and its later gzip — so a file is
// processed exactly once even after compression.
type LogFile struct {
	Path string
}

// RotatedFiles lists the rotated access-log files in dir — those whose name starts
// with base+"." (e.g. access.log.1, access.log.2.gz) — skipping the live `base`
// file, which is still being written. A missing directory yields an empty list and
// no error, so the worker is a clean no-op where no logs exist (local/dev). Results
// are sorted by name for deterministic processing.
func RotatedFiles(dir, base string) ([]LogFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	prefix := base + "."
	var files []LogFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		files = append(files, LogFile{Path: filepath.Join(dir, e.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// Open opens the file for reading, transparently decompressing a .gz. The returned
// closer closes both the gzip reader and the underlying file.
func (f LogFile) Open() (io.ReadCloser, error) {
	file, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(f.Path, ".gz") {
		return file, nil
	}
	zr, err := gzip.NewReader(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	return gzipReadCloser{zr: zr, file: file}, nil
}

// gzipReadCloser closes the gzip reader and the underlying file together.
type gzipReadCloser struct {
	zr   *gzip.Reader
	file *os.File
}

func (g gzipReadCloser) Read(p []byte) (int, error) { return g.zr.Read(p) }

func (g gzipReadCloser) Close() error {
	err := g.zr.Close()
	if cerr := g.file.Close(); err == nil {
		err = cerr
	}
	return err
}
