package telegram

import (
	"context"
	"errors"
	"testing"
)

// fakeRefilterStore is an in-memory telegram_posts holding only what the pass reads.
type fakeRefilterStore struct {
	posts     []StoredPost
	requeued  []string // "channel/msgid", in call order
	listCalls int
	raceOn    string // this key's requeue reports 0 rows, as if another worker took it
	listErr   error
}

func (s *fakeRefilterStore) ListRejected(_ context.Context, afterChannel string, afterMsgID int64, limit int32) ([]StoredPost, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []StoredPost
	for _, p := range s.posts {
		if p.Channel < afterChannel || (p.Channel == afterChannel && p.MsgID <= afterMsgID) {
			continue
		}
		out = append(out, p)
		if int32(len(out)) == limit {
			break
		}
	}
	return out, nil
}

func (s *fakeRefilterStore) Requeue(_ context.Context, channel string, msgID int64) (int64, error) {
	key := channel + "/" + itoa(msgID)
	if key == s.raceOn {
		return 0, nil
	}
	s.requeued = append(s.requeued, key)
	return 1, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// corpus: two posts today's markers admit, two they still refuse. Ordered by the
// primary key (channel, msg_id) the way the query returns them.
func refilterCorpus() []StoredPost {
	return []StoredPost{
		{Channel: "a_channel", MsgID: 1, Text: "Пятница! Всем хороших выходных 🎉"},
		{Channel: "a_channel", MsgID: 2, Text: "🧑‍💼 | DBA Oracle Senior\nEmpresa: Exceltec\nUbicación: Costa Rica"},
		{Channel: "b_channel", MsgID: 10, Text: "Дайджест новостей недели"},
		{Channel: "b_channel", MsgID: 11, Text: "Вакансия: Go разработчик в финтех"},
	}
}

func TestRefilterRunnerDryRunWritesNothing(t *testing.T) {
	store := &fakeRefilterStore{posts: refilterCorpus()}
	r := RefilterRunner{Store: store, Batch: 2}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.requeued) != 0 {
		t.Errorf("dry run wrote %v, want nothing", store.requeued)
	}
	if stats.Scanned != 4 {
		t.Errorf("Scanned = %d, want 4", stats.Scanned)
	}
	if stats.Admitted != 2 {
		t.Errorf("Admitted = %d, want 2", stats.Admitted)
	}
	if stats.Requeued != 0 {
		t.Errorf("Requeued = %d, want 0 in a dry run", stats.Requeued)
	}
	if stats.ByChannel["a_channel"] != 1 || stats.ByChannel["b_channel"] != 1 {
		t.Errorf("ByChannel = %v, want one per channel", stats.ByChannel)
	}
}

func TestRefilterRunnerApplyRequeuesOnlyAdmitted(t *testing.T) {
	store := &fakeRefilterStore{posts: refilterCorpus()}
	r := RefilterRunner{Store: store, Batch: 2, Apply: true}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"a_channel/2", "b_channel/11"}
	if len(store.requeued) != len(want) {
		t.Fatalf("requeued %v, want %v", store.requeued, want)
	}
	for i := range want {
		if store.requeued[i] != want[i] {
			t.Errorf("requeued[%d] = %s, want %s", i, store.requeued[i], want[i])
		}
	}
	if stats.Requeued != 2 {
		t.Errorf("Requeued = %d, want 2", stats.Requeued)
	}
}

// A post another worker claimed between the read and the write reports zero rows. It is
// not a failure and must not be counted as requeued — the guard in the UPDATE is what
// makes the pass safe to interrupt and re-run.
func TestRefilterRunnerDoesNotCountALostRace(t *testing.T) {
	store := &fakeRefilterStore{posts: refilterCorpus(), raceOn: "a_channel/2"}
	r := RefilterRunner{Store: store, Batch: 10, Apply: true}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Admitted != 2 {
		t.Errorf("Admitted = %d, want 2 — the race does not change what the filter said", stats.Admitted)
	}
	if stats.Requeued != 1 {
		t.Errorf("Requeued = %d, want 1", stats.Requeued)
	}
}

// Max bounds how many rows one run SCANS, not how many it requeues, so an operator can
// size a run against the clock without the bound depending on what the filter decides.
func TestRefilterRunnerMaxBoundsTheScan(t *testing.T) {
	store := &fakeRefilterStore{posts: refilterCorpus()}
	r := RefilterRunner{Store: store, Batch: 10, Max: 3}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3", stats.Scanned)
	}
	if !stats.Stopped {
		t.Error("Stopped = false, want true — a bounded run must say it did not finish")
	}
	if stats.NextChannel != "b_channel" || stats.NextMsgID != 10 {
		t.Errorf("resume point = %s/%d, want b_channel/10", stats.NextChannel, stats.NextMsgID)
	}
}

// The resume point a bounded run prints has to be usable, or it is decoration: refused
// posts never leave the predicate, so a second bounded run starting from the top rescans
// exactly what the first one rejected and a dry run repeats its report forever. This is
// the failure cmd/backfill-derive carried until freehire#2864, where afterID started at 0
// every time.
func TestRefilterRunnerResumesFromTheCursorItPrinted(t *testing.T) {
	store := &fakeRefilterStore{posts: refilterCorpus()}

	first, err := RefilterRunner{Store: store, Batch: 10, Max: 3}.Run(context.Background())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if !first.Stopped {
		t.Fatal("first run did not stop on its bound")
	}

	second, err := RefilterRunner{
		Store:        store,
		Batch:        10,
		AfterChannel: first.NextChannel,
		AfterMsgID:   first.NextMsgID,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Scanned != 1 {
		t.Errorf("second run scanned %d, want 1 — the 4th row is all that is left", second.Scanned)
	}
	if second.Admitted != 1 {
		t.Errorf("second run admitted %d, want 1", second.Admitted)
	}
	if first.Scanned+second.Scanned != 4 {
		t.Errorf("the two runs together scanned %d rows, want 4 — no row read twice, none skipped",
			first.Scanned+second.Scanned)
	}
}

// A run that reached the end says so, so "there is more" is never inferred from a count.
func TestRefilterRunnerReportsCompletion(t *testing.T) {
	store := &fakeRefilterStore{posts: refilterCorpus()}
	r := RefilterRunner{Store: store, Batch: 2}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Stopped {
		t.Error("Stopped = true, want false — this run read every row")
	}
}

func TestRefilterRunnerSurfacesAReadFailure(t *testing.T) {
	boom := errors.New("boom")
	store := &fakeRefilterStore{posts: refilterCorpus(), listErr: boom}
	r := RefilterRunner{Store: store, Batch: 2}

	if _, err := r.Run(context.Background()); !errors.Is(err, boom) {
		t.Errorf("Run error = %v, want %v", err, boom)
	}
}

// The pass must ask AdmitsPost, not LooksLikeVacancy: a stored post whose TEXT says
// nothing but whose stored links point at a destination adapter is admitted by the crawl
// and must be admitted here too.
func TestRefilterRunnerAdmitsOnStoredLinks(t *testing.T) {
	store := &fakeRefilterStore{posts: []StoredPost{
		{Channel: "c", MsgID: 1, Text: "Свежая подборка 👇", Links: []Link{{URL: "https://boards.greenhouse.io/acme/jobs/1"}}},
	}}
	r := RefilterRunner{Store: store, Batch: 10, Links: fakeMatcher{hit: true}, Apply: true}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Requeued != 1 {
		t.Errorf("Requeued = %d, want 1 — the stored link admits it", stats.Requeued)
	}
}

// A bound that happens to land exactly on the last row is not "there is more". Without a
// probe past the exhausted page the run reports Stopped and prints a resume cursor, and
// the operator spends a follow-up run to be told there was nothing — which is the same
// "inferred from a count" failure the completion report exists to remove.
func TestRefilterRunnerMaxLandingOnTheLastRowIsStillDone(t *testing.T) {
	corpus := refilterCorpus()
	store := &fakeRefilterStore{posts: corpus}
	r := RefilterRunner{Store: store, Batch: 10, Max: int64(len(corpus))}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Scanned != len(corpus) {
		t.Errorf("Scanned = %d, want %d", stats.Scanned, len(corpus))
	}
	if stats.Stopped {
		t.Error("Stopped = true, want false — the bound and the end of the table coincided")
	}
}
