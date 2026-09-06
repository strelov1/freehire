package mailingest

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// The daemon holds up to ten of these at once and has no MemoryMax, so the read has to be
// bounded — but a BARE LimitReader is the trap, not the fix: MIME headers are at the front,
// so a truncated message parses and would be stored as though it were whole.
func TestReadBoundedRefusesAnOversizeMessageRatherThanTruncatingIt(t *testing.T) {
	body := strings.NewReader(strings.Repeat("a", 101))

	got, err := readBounded(body, 100)
	if !errors.Is(err, errMessageTooLarge) {
		t.Fatalf("readBounded() err = %v, want errMessageTooLarge", err)
	}
	if got != nil {
		t.Errorf("readBounded() returned %d bytes alongside the error; half a message must "+
			"never reach the store as if it were whole", len(got))
	}
}

// One byte under the limit, and exactly at it, are both ordinary messages: the +1 read is
// what tells "at the limit" from "over it" without trusting a Content-Length.
func TestReadBoundedReturnsAMessageAtTheLimitWhole(t *testing.T) {
	for _, size := range []int{0, 99, 100} {
		want := bytes.Repeat([]byte("a"), size)
		got, err := readBounded(bytes.NewReader(want), 100)
		if err != nil {
			t.Fatalf("readBounded(%d bytes) err = %v", size, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("readBounded(%d bytes) returned %d bytes, want the message whole", size, len(got))
		}
	}
}

// A read that fails partway is not a size problem, and must not be reported as one: the
// caller acks and drops an oversize object, and doing that to a transient S3 error would
// discard a deliverable message.
func TestReadBoundedPropagatesAReadFailure(t *testing.T) {
	_, err := readBounded(io.MultiReader(strings.NewReader("part"), errReader{}), 100)
	if err == nil {
		t.Fatal("readBounded() over a failing reader returned no error")
	}
	if errors.Is(err, errMessageTooLarge) {
		t.Errorf("a read failure was reported as an oversize message (%v), which acks and drops it", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
