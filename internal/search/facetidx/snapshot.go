package facetidx

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"math"
	"slices"

	"github.com/RoaringBitmap/roaring/v2"
)

// Snapshot format, version 1 (all integers little-endian, uvarint where noted):
//
//	magic "FCTDSNAP", u32 version
//	uvarint n, then n x i64 job ids (doc id order)
//	closed, duplicate, deleted bitmaps
//	uvarint attribute count, then per attribute in indexedAttrs order:
//	  name, exists, empty, numeric bitmaps,
//	  uvarint value count, per value: display string, bitmap of documents,
//	  uvarint numeric entry count, per entry: f64 value, u32 doc (already sorted)
//	u32 CRC-32C of every byte above
//
// A bitmap is a uvarint length and roaring's portable serialization; a string is a uvarint
// length and its bytes. Not stored, because it is cheap to recompute: all, live, the id
// map, value keys, cardinalities and prefix sums, valued, and the forward arrays.
const (
	snapshotMagic   = "FCTDSNAP"
	snapshotVersion = 1
	maxStringLen    = 1 << 20
	maxBlob         = 1 << 30
)

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// ErrBadSnapshot is wrapped by every ReadSnapshot failure.
var ErrBadSnapshot = errors.New("facetidx: bad snapshot")

type snapWriter struct {
	w   io.Writer
	crc hash.Hash32
	buf [binary.MaxVarintLen64]byte
	err error
}

func (s *snapWriter) write(p []byte) {
	if s.err == nil {
		_, s.err = s.w.Write(p)
	}
}
func (s *snapWriter) uvarint(v uint64) { s.write(s.buf[:binary.PutUvarint(s.buf[:], v)]) }
func (s *snapWriter) u32(v uint32)     { binary.LittleEndian.PutUint32(s.buf[:], v); s.write(s.buf[:4]) }
func (s *snapWriter) u64(v uint64)     { binary.LittleEndian.PutUint64(s.buf[:], v); s.write(s.buf[:8]) }
func (s *snapWriter) str(v string)     { s.uvarint(uint64(len(v))); s.write([]byte(v)) }
func (s *snapWriter) bitmap(b *roaring.Bitmap) {
	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil && s.err == nil {
		s.err = err
	}
	s.uvarint(uint64(buf.Len()))
	s.write(buf.Bytes())
}

// WriteSnapshot writes the index in the snapshot format.
func (ix *Index) WriteSnapshot(w io.Writer) error {
	bw := bufio.NewWriterSize(w, 1<<20)
	crc := crc32.New(crcTable)
	s := &snapWriter{w: io.MultiWriter(bw, crc), crc: crc}
	s.write([]byte(snapshotMagic))
	s.u32(snapshotVersion)
	s.uvarint(uint64(ix.n))
	for _, id := range ix.jobIDs {
		s.u64(uint64(id))
	}
	s.bitmap(ix.closed)
	s.bitmap(ix.dup)
	s.bitmap(ix.deleted)
	s.uvarint(uint64(len(indexedAttrs)))
	for _, name := range indexedAttrs {
		f := ix.fields[name]
		s.str(name)
		s.bitmap(f.exists)
		s.bitmap(f.empty)
		s.bitmap(f.numeric)
		s.uvarint(uint64(len(f.vals)))
		for i, v := range f.vals {
			s.str(v)
			s.bitmap(f.bms[i])
		}
		s.uvarint(uint64(len(f.num)))
		for _, e := range f.num {
			s.u64(math.Float64bits(e.v))
			s.u32(e.doc)
		}
	}
	if s.err != nil {
		return s.err
	}
	sum := crc.Sum32()
	var tail [4]byte
	binary.LittleEndian.PutUint32(tail[:], sum)
	if _, err := bw.Write(tail[:]); err != nil {
		return err
	}
	return bw.Flush()
}

type snapReader struct {
	r   *bufio.Reader
	crc hash.Hash32
	err error
}

func (s *snapReader) fail(format string, a ...any) {
	if s.err == nil {
		s.err = fmt.Errorf("%w: "+format, append([]any{ErrBadSnapshot}, a...)...)
	}
}

func (s *snapReader) read(p []byte) {
	if s.err != nil {
		return
	}
	if _, err := io.ReadFull(s.r, p); err != nil {
		s.fail("%v", err)
		return
	}
	s.crc.Write(p)
}

func (s *snapReader) uvarint() uint64 {
	if s.err != nil {
		return 0
	}
	var b [binary.MaxVarintLen64]byte
	for i := range b {
		c, err := s.r.ReadByte()
		if err != nil {
			s.fail("%v", err)
			return 0
		}
		b[i] = c
		s.crc.Write(b[i : i+1])
		if c < 0x80 {
			v, _ := binary.Uvarint(b[:i+1])
			return v
		}
	}
	s.fail("varint overflow")
	return 0
}

func (s *snapReader) u32() uint32 {
	var b [4]byte
	s.read(b[:])
	return binary.LittleEndian.Uint32(b[:])
}

func (s *snapReader) u64() uint64 {
	var b [8]byte
	s.read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

// count reads a length and rejects one no honest snapshot could carry.
func (s *snapReader) count(limit uint64) int {
	v := s.uvarint()
	if v > limit {
		s.fail("length %d exceeds %d", v, limit)
		return 0
	}
	return int(v)
}

func (s *snapReader) blob(limit uint64) []byte {
	n := s.count(limit)
	if s.err != nil {
		return nil
	}
	// grow as bytes arrive, so a corrupt length cannot allocate more than the file holds
	out := make([]byte, 0, min(n, 1<<16))
	for len(out) < n && s.err == nil {
		chunk := make([]byte, min(n-len(out), 1<<20))
		s.read(chunk)
		out = append(out, chunk...)
	}
	return out
}

func (s *snapReader) str() string { return string(s.blob(maxStringLen)) }

func (s *snapReader) bitmap() *roaring.Bitmap {
	b := roaring.New()
	raw := s.blob(maxBlob)
	if s.err == nil {
		if _, err := b.ReadFrom(bytes.NewReader(raw)); err != nil {
			s.fail("bitmap: %v", err)
		}
	}
	return b
}

// ReadSnapshot restores an index written by WriteSnapshot. Any damage (wrong magic or
// version, truncation, a bad checksum) is an error wrapping ErrBadSnapshot; no index is
// returned in that case.
func ReadSnapshot(r io.Reader) (ix *Index, err error) {
	defer func() {
		if p := recover(); p != nil { // a hostile length inside roaring's decoder
			ix, err = nil, fmt.Errorf("%w: %v", ErrBadSnapshot, p)
		}
	}()
	s := &snapReader{r: bufio.NewReaderSize(r, 1<<20), crc: crc32.New(crcTable)}
	magic := make([]byte, len(snapshotMagic))
	s.read(magic)
	if s.err == nil && string(magic) != snapshotMagic {
		s.fail("not a snapshot")
	}
	if v := s.u32(); s.err == nil && v != snapshotVersion {
		s.fail("format version %d, want %d", v, snapshotVersion)
	}
	n := s.count(math.MaxUint32)
	ix = &Index{n: uint32(n), docOf: map[int64]uint32{}, fields: map[string]*field{}}
	for i := 0; i < n && s.err == nil; i++ {
		id := int64(s.u64())
		if _, dup := ix.docOf[id]; dup {
			s.fail("duplicate job id %d", id)
			break
		}
		ix.docOf[id] = uint32(i)
		ix.jobIDs = append(ix.jobIDs, id)
	}
	ix.closed, ix.dup, ix.deleted = s.bitmap(), s.bitmap(), s.bitmap()
	if got := s.count(1 << 16); s.err == nil && got != len(indexedAttrs) {
		s.fail("%d attributes, want %d", got, len(indexedAttrs))
	}
	for _, want := range indexedAttrs {
		if s.err != nil {
			break
		}
		if name := s.str(); s.err == nil && name != want {
			s.fail("attribute %q, want %q", name, want)
			break
		}
		f := &field{exists: s.bitmap(), empty: s.bitmap(), numeric: s.bitmap(), keys: map[string]uint32{}}
		nv := s.count(math.MaxUint32)
		for i := 0; i < nv && s.err == nil; i++ {
			v := s.str()
			f.keys[norm(v)] = uint32(i)
			f.vals = append(f.vals, v)
			f.bms = append(f.bms, s.bitmap())
		}
		nn := s.count(math.MaxUint32)
		for i := 0; i < nn && s.err == nil; i++ {
			v := math.Float64frombits(s.u64())
			f.num = append(f.num, numEntry{v, s.u32()})
		}
		ix.fields[want] = f
	}
	if s.err != nil {
		return nil, s.err
	}
	want := s.crc.Sum32()
	var tail [4]byte
	if _, err := io.ReadFull(s.r, tail[:]); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadSnapshot, err)
	}
	if binary.LittleEndian.Uint32(tail[:]) != want {
		return nil, fmt.Errorf("%w: checksum mismatch", ErrBadSnapshot)
	}
	ix.derive()
	return ix, nil
}

// derive recomputes everything the snapshot does not hold.
func (ix *Index) derive() {
	ix.all = roaring.New()
	if ix.n > 0 {
		ix.all.AddRange(0, uint64(ix.n))
	}
	ix.live = roaring.AndNot(roaring.AndNot(roaring.AndNot(ix.all, ix.closed), ix.dup), ix.deleted)
	for _, f := range ix.fields {
		f.cards = make([]uint32, len(f.bms))
		f.valued = roaring.New()
		for i, bm := range f.bms {
			f.cards[i] = uint32(bm.GetCardinality())
			f.postings += uint64(f.cards[i])
			f.valued.Or(bm)
		}
		f.withVals = f.valued.GetCardinality()
		slices.Sort(f.cards)
		f.cardSum = make([]uint64, len(f.cards)+1)
		for i, c := range f.cards {
			f.cardSum[i+1] = f.cardSum[i] + uint64(c)
		}
		if f.wantsForward() {
			f.buildForward(ix.n)
		}
	}
}
