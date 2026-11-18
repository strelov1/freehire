package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"os"
	"time"

	"github.com/strelov1/freehire/internal/search/facetidx"
)

// Write-ahead log: every accepted /write batch is appended and fsynced before it is applied
// and before the 200 reply, so an acknowledged batch survives a crash between snapshots.
//
// Record: [4-byte big-endian payload length][4-byte CRC-32 (IEEE) of the payload][payload],
// the payload being the /write request body as JSON (writeReq).
//
// Replay rule: on start, load the snapshot, then apply EVERY record in the file in order.
// A record already contained in the snapshot is replayed again, which is harmless: a batch is
// a set of per-job full replaces and marks, so the last operation per job wins either way.
// A snapshot rotates the log: records appended after the snapshot's index was captured
// (offset walOff at that instant) are kept, everything before is dropped, via temp file +
// rename, so an acknowledged batch is always in the snapshot or in the log.
//
// A torn last record (crash mid-append) is ignored; a bad record anywhere else fails the
// recovery, moves the log aside as *.corrupt-<unix> and leaves the server not-ready.

const walHeader = 8

func (r writeReq) batch() facetidx.Batch {
	b := facetidx.Batch{Close: r.Close, Duplicate: r.Duplicate, Delete: r.Delete}
	for _, u := range r.Upserts {
		b.Upserts = append(b.Upserts, u)
	}
	return b
}

func encodeWALRecord(req writeReq) ([]byte, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	// scanWAL treats a longer record as corruption, so refuse it here rather than acknowledge
	// a batch the replay would drop.
	if len(payload) > maxBulkBody {
		return nil, fmt.Errorf("wal: record of %d bytes exceeds %d", len(payload), maxBulkBody)
	}
	rec := make([]byte, walHeader, walHeader+len(payload))
	binary.BigEndian.PutUint32(rec[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(rec[4:8], crc32.ChecksumIEEE(payload))
	return append(rec, payload...), nil
}

// scanWAL returns the decoded records and the offset just after the last good one. A torn
// tail (incomplete, or a bad checksum on the final record) stops the scan cleanly; anything
// bad before the final record is an error.
func scanWAL(data []byte) (recs []writeReq, good int, err error) {
	for off := 0; off < len(data); {
		if len(data)-off < walHeader {
			return recs, off, nil // torn header
		}
		n := int(binary.BigEndian.Uint32(data[off : off+4]))
		sum := binary.BigEndian.Uint32(data[off+4 : off+8])
		end := off + walHeader + n
		if n > maxBulkBody { // the writer never writes one: a damaged header, wherever it is
			return nil, 0, fmt.Errorf("wal: record at %d claims %d bytes", off, n)
		}
		if end > len(data) {
			return recs, off, nil // torn payload
		}
		payload := data[off+walHeader : end]
		var req writeReq
		if crc32.ChecksumIEEE(payload) != sum || json.Unmarshal(payload, &req) != nil {
			if end == len(data) {
				return recs, off, nil // torn final record
			}
			return nil, 0, fmt.Errorf("wal: corrupt record at offset %d", off)
		}
		recs = append(recs, req)
		off = end
	}
	return recs, len(data), nil
}

// openWAL reads the log (if any), replays it into the index loaded from the snapshot (nil =
// no snapshot) and opens it for appending. It returns the replayed store, nil when there is
// no snapshot. Callers hold no locks: it runs before the server takes traffic.
func (s *Server) openWAL(ix *facetidx.Index) (*facetidx.Store, error) {
	if s.walPath == "" {
		if ix == nil {
			return nil, nil
		}
		return facetidx.NewStore(ix), nil
	}
	data, err := os.ReadFile(s.walPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.m.walErrors.Inc()
		return nil, err
	}
	recs, good, err := scanWAL(data)
	if err != nil {
		s.m.walErrors.Inc()
		aside := fmt.Sprintf("%s.corrupt-%d", s.walPath, time.Now().Unix())
		if rerr := os.Rename(s.walPath, aside); rerr != nil {
			return nil, fmt.Errorf("%w (and moving it aside failed: %v)", err, rerr)
		}
		log.Printf("facetd: WAL corrupt, moved to %s; staying not-ready", aside)
		_ = s.openWALFile()
		return nil, fmt.Errorf("%w; moved to %s", err, aside)
	}
	if good < len(data) {
		log.Printf("facetd: WAL: ignoring a torn last record (%d trailing bytes)", len(data)-good)
	}
	if err := s.openWALFile(); err != nil {
		return nil, err
	}
	if good < len(data) {
		if err := s.truncateWAL(int64(good)); err != nil {
			return nil, err
		}
	}
	s.walOff, s.walRecs = int64(good), len(recs)
	s.m.walBytes.Set(float64(good))
	s.m.walRecords.Set(float64(len(recs)))

	if ix == nil {
		if len(recs) > 0 {
			log.Printf("facetd: WAL has %d records but there is no snapshot; the log alone is not an index, staying not-ready", len(recs))
		}
		return nil, nil
	}
	st := facetidx.NewStore(ix)
	for _, r := range recs {
		if _, err := st.Apply(r.batch()); err != nil {
			s.m.walErrors.Inc()
			return nil, fmt.Errorf("wal replay: %w", err)
		}
		s.m.walReplayed.Inc()
	}
	if len(recs) > 0 {
		log.Printf("facetd: WAL replayed %d records", len(recs))
	}
	return st, nil
}

func (s *Server) openWALFile() error {
	f, err := os.OpenFile(s.walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if s.wal != nil {
		_ = s.wal.Close()
	}
	s.wal = f
	return nil
}

func (s *Server) truncateWAL(size int64) error {
	if err := s.wal.Truncate(size); err != nil {
		return err
	}
	return s.wal.Sync()
}

// logWrite appends and fsyncs one batch; s.mu is held. On failure the file is cut back to
// the last good record so a half-written record never sits in front of later ones.
func (s *Server) logWrite(req writeReq) (int64, error) {
	if s.walPath == "" {
		return 0, nil
	}
	if s.walBroken {
		return 0, errors.New("wal unusable after an earlier failure")
	}
	if s.wal == nil {
		if err := s.openWALFile(); err != nil { // server used without LoadSnapshot
			s.m.walErrors.Inc()
			return 0, err
		}
		fi, err := s.wal.Stat()
		if err != nil {
			s.m.walErrors.Inc()
			return 0, err
		}
		s.walOff = fi.Size()
	}
	rec, err := encodeWALRecord(req)
	if err == nil {
		_, err = s.wal.Write(rec)
	}
	if err == nil {
		err = s.wal.Sync()
	}
	if err != nil {
		s.m.walErrors.Inc()
		s.walRollback()
		return 0, err
	}
	s.walOff += int64(len(rec))
	s.walRecs++
	s.m.walBytes.Set(float64(s.walOff))
	s.m.walRecords.Set(float64(s.walRecs))
	return int64(len(rec)), nil
}

// walRollback drops everything after walOff (a failed append, or a record whose batch the
// index then refused). If that fails the log can no longer be trusted: refuse further writes.
func (s *Server) walRollback() {
	if err := s.truncateWAL(s.walOff); err != nil {
		s.walBroken = true
		log.Printf("facetd: WAL rollback failed, refusing writes: %v", err)
	}
}

// walUndo removes the record logWrite just appended (its batch was refused by the index).
func (s *Server) walUndo(recLen int64) {
	if recLen == 0 {
		return
	}
	s.walOff -= recLen
	s.walRecs--
	s.walRollback()
	s.m.walBytes.Set(float64(s.walOff))
	s.m.walRecords.Set(float64(s.walRecs))
}

// rotateWAL keeps only the bytes appended after offset `from` (the log position when the
// just-written snapshot's index was captured). Called after the snapshot rename.
func (s *Server) rotateWAL(from int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.walPath == "" || s.wal == nil || from <= 0 {
		return nil
	}
	tail := make([]byte, s.walOff-from)
	if _, err := s.wal.ReadAt(tail, from); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	_, err := writeFileAtomic(s.walPath, func(w io.Writer) error {
		_, err := io.Copy(w, bytes.NewReader(tail))
		return err
	})
	// After the rename the new file is in place even if syncing its directory failed: the
	// handle must follow it, or later acknowledged writes would land on the unlinked inode.
	var renamed postRenameError
	if err != nil && !errors.As(err, &renamed) {
		return err
	}
	// The old handle still points at the replaced inode; reopen on the new file.
	if oerr := s.openWALFile(); oerr != nil {
		s.walBroken = true
		return oerr
	}
	s.walOff = int64(len(tail))
	kept, _, _ := scanWAL(tail)
	s.walRecs = len(kept)
	s.m.walBytes.Set(float64(s.walOff))
	s.m.walRecords.Set(float64(s.walRecs))
	return err
}
