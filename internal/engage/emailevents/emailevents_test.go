package emailevents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

const openEvent = `{"eventType":"Open","mail":{"messageId":"m-1","timestamp":"2026-11-10T14:00:00Z","destination":["person@example.test"],"tags":{"campaign":["launch"],"variant":["a"],"ses:from-domain":["freehire.me"]}},"open":{"timestamp":"2026-11-10T15:30:00Z","ipAddress":"203.0.113.9"}}`

func snsWrapped(t *testing.T, event string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"Type": "Notification", "Message": event})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDecode_ReadsAnEventBothWrappedAndRaw(t *testing.T) {
	want := db.RecordEmailEventParams{
		MessageID: "m-1", Type: "open", Campaign: "launch", Variant: "a",
		OccurredAt: pgtype.Timestamptz{Time: time.Date(2026, 11, 10, 15, 30, 0, 0, time.UTC), Valid: true},
	}
	for name, body := range map[string]string{"sns": snsWrapped(t, openEvent), "raw": openEvent} {
		got, err := decode(body)
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if !got.OccurredAt.Time.Equal(want.OccurredAt.Time) {
			t.Errorf("%s: occurred at %v, want %v", name, got.OccurredAt.Time, want.OccurredAt.Time)
		}
		got.OccurredAt = want.OccurredAt
		if got != want {
			t.Errorf("%s: decoded %+v, want %+v", name, got, want)
		}
	}
}

func TestDecode_SendFallsOnTheMailTimestamp(t *testing.T) {
	got, err := decode(`{"eventType":"Send","mail":{"messageId":"m","timestamp":"2026-11-10T14:00:00Z","tags":{"campaign":["c"]}},"send":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "send" || got.OccurredAt.Time.Hour() != 14 {
		t.Errorf("decoded %+v", got)
	}
}

func TestDecode_IgnoresWhatTheLedgerDoesNotTrack(t *testing.T) {
	for name, body := range map[string]string{
		"no campaign tag": `{"eventType":"Open","mail":{"messageId":"m","tags":{}}}`,
		"untracked type":  `{"eventType":"Rendering Failure","mail":{"messageId":"m","tags":{"campaign":["c"]}}}`,
	} {
		if _, err := decode(body); !errors.Is(err, errIgnored) {
			t.Errorf("%s: err = %v, want errIgnored", name, err)
		}
	}
}

type fakeSource struct {
	batches [][]Received
	acked   []string
}

func (s *fakeSource) Receive(context.Context) ([]Received, error) {
	if len(s.batches) == 0 {
		return nil, nil
	}
	b := s.batches[0]
	s.batches = s.batches[1:]
	return b, nil
}

func (s *fakeSource) Ack(_ context.Context, handle string) error {
	s.acked = append(s.acked, handle)
	return nil
}

type fakeStore struct {
	recorded []db.RecordEmailEventParams
	err      error
	rollUps  []int64
	before   time.Time
}

func (f *fakeStore) RecordEmailEvent(_ context.Context, arg db.RecordEmailEventParams) error {
	if f.err != nil {
		return f.err
	}
	f.recorded = append(f.recorded, arg)
	return nil
}

func (f *fakeStore) RollUpExpiredEmailEvents(_ context.Context, arg db.RollUpExpiredEmailEventsParams) (int64, error) {
	f.before = arg.Before.Time
	n := f.rollUps[0]
	f.rollUps = f.rollUps[1:]
	return n, nil
}

func TestDrain_RecordsAndAcksEverythingItCannotUseEither(t *testing.T) {
	src := &fakeSource{batches: [][]Received{{
		{Body: openEvent, AckHandle: "good"},
		{Body: "not json", AckHandle: "broken"},
		{Body: `{"eventType":"Open","mail":{"messageId":"m","tags":{}}}`, AckHandle: "untagged"},
	}}}
	store := &fakeStore{}

	stats, err := Drain(context.Background(), src, store)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if stats != (Stats{Recorded: 1, Ignored: 1, Failed: 1}) {
		t.Errorf("stats = %+v", stats)
	}
	if len(src.acked) != 3 {
		t.Errorf("acked %v, want all three", src.acked)
	}
}

func TestDrain_LeavesAFailedWriteOnTheQueue(t *testing.T) {
	src := &fakeSource{batches: [][]Received{{{Body: openEvent, AckHandle: "good"}}}}
	if _, err := Drain(context.Background(), src, &fakeStore{err: errors.New("db down")}); err == nil {
		t.Fatal("Drain: want an error")
	}
	if len(src.acked) != 0 {
		t.Errorf("acked %v: a failed write would be lost", src.acked)
	}
}

func TestRollUp_RunsUntilNothingIsPastRetention(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{rollUps: []int64{3, 1, 0}}
	if err := RollUp(context.Background(), store, now); err != nil {
		t.Fatalf("RollUp: %v", err)
	}
	if len(store.rollUps) != 0 {
		t.Errorf("%d passes left unrun", len(store.rollUps))
	}
	if want := now.Add(-Retention); !store.before.Equal(want) {
		t.Errorf("before = %v, want %v", store.before, want)
	}
}
