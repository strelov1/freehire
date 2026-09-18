package mailingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

// SES answers "who was this addressed to" twice, and only one of the two answers is the
// address the message was DELIVERED to. mail.destination is built from the To/Cc headers;
// receipt.recipients is the envelope RCPT TO that matched the receipt rule, and AWS says
// in as many words that the two may differ. Reading the headers dropped every forwarded
// message — a Sieve `redirect` rewrites the envelope and leaves the headers naming the
// original mailbox — and, worse, let a sender choose whose inbox a message landed in by
// writing another user's address into To:, since the header is the sender's to write.
func TestDecodeNotificationReadsTheEnvelopeRecipientNotTheHeaderOnes(t *testing.T) {
	note, err := decodeNotification(sesNotificationJSON(t,
		[]string{"freehire-2@mail.freehire.me"}, // envelope: forwarded here
		[]string{"test@hiring.klavaro.net"},     // headers: the original mailbox
	))
	if err != nil {
		t.Fatalf("decodeNotification() err = %v", err)
	}
	if want := []string{"freehire-2@mail.freehire.me"}; !slices.Equal(note.Receipt.Recipients, want) {
		t.Errorf("recipients = %v, want the envelope recipients %v", note.Receipt.Recipients, want)
	}
}

// sesNotificationJSON builds the SNS-wrapped SES "Received" notification, carrying the two
// recipient lists separately so a test can make them disagree.
func sesNotificationJSON(t *testing.T, envelope, headers []string) string {
	t.Helper()
	inner, err := json.Marshal(map[string]any{
		"notificationType": "Received",
		"mail":             map[string]any{"destination": headers},
		"receipt": map[string]any{
			"recipients": envelope,
			"action": map[string]any{
				"type": "S3", "bucketName": "raw", "objectKey": "inbound/abc",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	outer, err := json.Marshal(map[string]any{"Type": "Notification", "Message": string(inner)})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return string(outer)
}

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
