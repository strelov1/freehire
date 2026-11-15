package catalogexport

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/strelov1/freehire/internal/platform/blobstore"
)

// Batch returns the next lines (JSON objects, no trailing newline) after the cursor it was
// last given, and the new cursor. No lines ends the export. A cursor is the caller's own
// keyset position (a job id), so memory stays one batch whatever the catalogue's size.
type Batch func(ctx context.Context, after int64) (lines [][]byte, next int64, err error)

// ErrPublished is returned by Write, with the day's manifest, when the day already has one.
var ErrPublished = errors.New("catalogexport: snapshot already published")

// Write streams every batch through gzip into the bucket as the snapshot of day, then writes
// the manifest. A published day is never rewritten: a client may hold its checksum and a
// download link, so a rerun returns ErrPublished instead. A failure leaves no manifest, so an
// unfinished day is neither listed nor counted.
func Write(ctx context.Context, b blobstore.Bucket, day string, now time.Time, next Batch) (Manifest, error) {
	if m, err := readManifest(ctx, b, day); err == nil {
		return m, ErrPublished
	} else if !blobstore.IsNotFound(err) {
		return Manifest{}, fmt.Errorf("catalogexport: check %s: %w", day, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := b.PutStream(ctx, DataKey(day), "application/gzip", pr)
		pr.CloseWithError(err) // unblock the producer if the upload died first
		done <- err
	}()

	sum := sha256.New()
	counter := &countWriter{w: io.MultiWriter(pw, sum)}
	rows, err := produce(ctx, counter, next)
	if err != nil {
		pw.CloseWithError(err)
		return Manifest{}, errors.Join(err, <-done)
	}
	_ = pw.Close()
	if err := <-done; err != nil {
		return Manifest{}, err
	}

	m := Manifest{
		SchemaVersion: SchemaVersion, Date: day, Rows: rows, Bytes: counter.n,
		SHA256: hex.EncodeToString(sum.Sum(nil)), GeneratedAt: now.UTC(),
	}
	body, err := json.Marshal(m)
	if err != nil {
		return Manifest{}, err
	}
	if err := b.Put(ctx, ManifestKey(day), "application/json", bytes.NewReader(body), int64(len(body))); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func produce(ctx context.Context, w io.Writer, next Batch) (int64, error) {
	gz := gzip.NewWriter(w)
	var rows, after int64
	for {
		lines, cursor, err := next(ctx, after)
		if err != nil {
			return 0, err
		}
		if len(lines) == 0 {
			break
		}
		for _, l := range lines {
			if _, err := gz.Write(append(l, '\n')); err != nil {
				return 0, fmt.Errorf("catalogexport: write: %w", err)
			}
		}
		rows += int64(len(lines))
		after = cursor
	}
	if err := gz.Close(); err != nil {
		return 0, fmt.Errorf("catalogexport: close gzip: %w", err)
	}
	return rows, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
