# Screening Answer Bank Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an answer the candidate gives once to an employer's screening question serve every later application that asks the same thing, however that employer words it.

**Architecture:** A new `screening_answer_bank` table keyed by `(user_id, topic)`, where topic comes from a pure fold of the question text (dictionary first, normalised text as fallback). `candidateprofile.Assembler` merges banked answers into the same `map[string]string` the resolver already consumes, under `topic:` prefixed keys, so `resolve.go` gains one lookup and no new dependency on the store. Provenance follows `internal/candidate/experience`: only an answer the candidate themselves gave may reach an employer.

**Tech Stack:** Go 1.26, Fiber v2, PostgreSQL + sqlc, SvelteKit 5 (runes), vitest.

## Global Constraints

- **English only** in code, comments, identifiers, docs and commit messages.
- **Migration number: 0155.** `0154_auto_apply_queue_tailor_failed_at.sql` is taken by PR #2698 (open at the time of writing). If that PR merged with a different number, re-check `ls migrations/ | tail -3` and renumber before starting Task 2. Never edit an applied migration.
- **`make sqlc` after every `internal/platform/db/queries/*.sql` edit.** The pre-commit hook and CI both regenerate and diff.
- **Before committing any `*.go`:** `gofmt -w` those paths, then `go vet ./...` and `go test ./...`. Before pushing: `go vet -tags=integration ./...`.
- **A new package must be added to `internal/platform/arch/layering/blocks.go`** or the layering guard fails. `answerbank` goes in the `candidate` block's list (beside `"experience"`, line ~132); `answertopic` goes in the `dict` block's list.
- **Answer length ceiling: 2000 characters**, enforced in the service (an API key reaches the route too, not only the form).
- **Never relax `sensitive.go`.** This feature adds a source of *the candidate's own* answers. It must not make a model's guess sendable.
- The repo runs a ratchet lint policy: only issues new-from-main fail. Pre-existing warnings in untouched files are not yours to fix.

---

## File Structure

**Create:**
- `internal/dict/answertopic/answertopic.go` — the fold and the topic dictionary. Pure, no I/O.
- `internal/dict/answertopic/answertopic_test.go`
- `internal/dict/answertopic/AGENTS.md` — what a topic is and why the dictionary is not the only route.
- `internal/candidate/answerbank/answerbank.go` — `Answer`, `Provenance`, `Store`, the send-gate.
- `internal/candidate/answerbank/answerbank_test.go`
- `internal/candidate/answerbank/repository.go` — the `Repository` interface and its `db.Queries` implementation.
- `internal/candidate/answerbank/AGENTS.md`
- `migrations/0155_screening_answer_bank.sql`
- `internal/platform/db/queries/screening_answer_bank.sql`
- `internal/api/handler/me_answer_bank.go` — the three routes.
- `internal/api/handler/me_answer_bank_integration_test.go`
- `web/src/lib/answerBank.ts` — the pure "which pending questions are answerable here" helper.
- `web/src/lib/answerBank.test.ts`

**Modify:**
- `internal/platform/arch/layering/blocks.go` — register both new packages.
- `internal/api/candidateprofile/profile.go` — a new nil-able `BankReader` source; `Profile.BankAnswers`; `Fields()` merges them under `topic:` keys.
- `internal/api/atsapply/resolve.go` — `resolveOne` falls back to a topic lookup.
- `internal/api/atsapply/resolve_test.go` — the fallback's own tests.
- `internal/api/handler/handler.go` — construct the store, pass it to `NewAssembler`.
- `cmd/auto-apply/main.go` — same construction for the worker's own assembler.
- `web/src/lib/api.ts` — the three client calls.
- `web/src/lib/types.ts` — `BankedAnswer`.
- `web/src/lib/components/JobDrawer.svelte` — an input beside each pending question.

---

### Task 1: The topic fold

**Files:**
- Create: `internal/dict/answertopic/answertopic.go`
- Create: `internal/dict/answertopic/answertopic_test.go`
- Create: `internal/dict/answertopic/AGENTS.md`
- Modify: `internal/platform/arch/layering/blocks.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `answertopic.Of(question string) (topic string, ok bool)` — `ok` is false when the question folds to nothing (a label of pure punctuation), which callers must refuse rather than store.

- [ ] **Step 1: Write the failing test**

Create `internal/dict/answertopic/answertopic_test.go`:

```go
package answertopic

import "testing"

// The three phrasings a salary question actually arrives in. They must key alike, or the
// candidate answers the same question once per employer — the whole point of the bank.
func TestOf_SalaryPhrasingsShareATopic(t *testing.T) {
	first, ok := Of("What is your desired salary?")
	if !ok {
		t.Fatal("Of refused a well-formed question")
	}
	for _, phrasing := range []string{
		"Desired salary",
		"  desired   SALARY  ",
		"Please tell us your desired salary.",
	} {
		got, ok := Of(phrasing)
		if !ok {
			t.Fatalf("Of(%q) refused a well-formed question", phrasing)
		}
		if got != first {
			t.Errorf("Of(%q) = %q, want %q — the same question phrased differently", phrasing, got, first)
		}
	}
}

// Two genuinely different questions must NOT collapse: a bank that merges them answers one
// with the other, in the candidate's name, to an employer.
func TestOf_DifferentQuestionsKeepDifferentTopics(t *testing.T) {
	desired, _ := Of("What is your desired salary?")
	current, _ := Of("What is your current salary?")
	if desired == current {
		t.Errorf("desired and current salary both key to %q — answering one with the other misreports the candidate", desired)
	}
}

// The dictionary is an accelerator, not the only route: a question it has never heard of
// still gets a stable key from the fold alone.
func TestOf_AnUnknownQuestionStillKeysStably(t *testing.T) {
	first, ok := Of("Which state do you currently reside in?")
	if !ok {
		t.Fatal("Of refused a well-formed question the dictionary does not know")
	}
	again, _ := Of("which state do you currently reside in")
	if first != again {
		t.Errorf("Of is not stable for an unknown question: %q vs %q", first, again)
	}
}

// Greenhouse's hidden proxy inputs reach this with an empty label (see
// internal/api/atsapply/domscan.go). A key derived from nothing can never be recalled, so
// it must be refused rather than stored.
func TestOf_RefusesAQuestionThatFoldsToNothing(t *testing.T) {
	for _, empty := range []string{"", "   ", "???", "-- --"} {
		if got, ok := Of(empty); ok {
			t.Errorf("Of(%q) = %q, true; want a refusal — nothing can be recalled by this key", empty, got)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/dict/answertopic/`
Expected: FAIL — `undefined: Of` (the package does not exist yet).

- [ ] **Step 3: Write minimal implementation**

Create `internal/dict/answertopic/answertopic.go`:

```go
// Package answertopic keys an employer's screening question by WHAT IT ASKS rather than
// how it is worded, so an answer the candidate gives once serves every later posting that
// asks the same thing.
//
// Two passes, in order, and the order is the point. A dictionary of known topics runs
// first because it collapses phrasings a fold cannot ("compensation expectations" and
// "desired salary" share no words). Everything else falls back to a fold of the question
// text — so a question nobody anticipated still gets a stable key, and a missing
// dictionary entry costs a coarser key rather than a lost answer.
//
// That fallback is why this is not another curated list. A hand-maintained list that is
// the SOLE route is a list whose gaps are invisible: internal/api/atsapply's own
// labelAnswerKeyFor had no salary rule for months, and nothing reported the questions it
// was failing to match.
//
// Pure and deterministic — no model, no I/O. A model proposing a topic for an awkwardly
// folded question is deliberately out of scope.
package answertopic

import "strings"

// dictionary maps a topic to the keyword sets that name it. ALL keywords in a set must
// appear for that set to fire; any one set firing is enough.
//
// Kept small on purpose: it exists for phrasings the fold genuinely cannot unify, not as
// the place every question is expected to be listed.
var dictionary = []struct {
	topic string
	sets  [][]string
}{
	{"salary_expectation", [][]string{
		{"desired", "salary"},
		{"desired", "compensation"},
		{"salary", "expect"},
		{"compensation", "expect"},
	}},
	{"notice_period", [][]string{
		{"notice", "period"},
		{"how much notice"},
	}},
	{"relocation", [][]string{
		{"relocat"},
	}},
	{"start_date", [][]string{
		{"start", "date"},
		{"when can you start"},
	}},
}

// politeWrappers are the openers employers put in front of the actual question. Dropped so
// "Please tell us your desired salary" and "Desired salary" key alike.
var politeWrappers = []string{
	"please tell us",
	"please share",
	"please provide",
	"could you tell us",
	"could you share",
	"we would like to know",
	"we'd like to know",
	"tell us",
}

// Of returns the topic a question is keyed by, and whether it could be keyed at all.
//
// A question that folds to nothing — an empty label, or one that is pure punctuation, both
// of which real forms produce — is refused. A key derived from nothing cannot be recalled,
// so storing one would silently mark a question answered forever.
func Of(question string) (string, bool) {
	folded := fold(question)
	if folded == "" {
		return "", false
	}
	for _, entry := range dictionary {
		for _, set := range entry.sets {
			if containsAll(folded, set) {
				return entry.topic, true
			}
		}
	}
	return folded, true
}

// fold normalises a question to its comparable form: lowercase, letters and digits only
// (every other rune becomes a space), single-spaced, with a leading politeness wrapper
// dropped.
func fold(question string) string {
	lower := strings.ToLower(question)
	var b strings.Builder
	b.Grow(len(lower))
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			// Every non-alphanumeric rune, not a fixed punctuation list: a question can
			// carry a currency symbol, an em dash or a non-Latin quote, and naming them
			// one at a time is the curated-list trap this package exists to avoid.
			b.WriteRune(' ')
		}
	}
	folded := strings.Join(strings.Fields(b.String()), " ")
	for _, wrapper := range politeWrappers {
		if strings.HasPrefix(folded, wrapper+" ") {
			folded = strings.TrimSpace(strings.TrimPrefix(folded, wrapper))
			break
		}
	}
	return folded
}

// containsAll reports whether every keyword appears in the folded question.
func containsAll(folded string, keywords []string) bool {
	for _, kw := range keywords {
		if !strings.Contains(folded, kw) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/dict/answertopic/ -v`
Expected: PASS, all four tests.

- [ ] **Step 5: Register the package with the layering guard**

In `internal/platform/arch/layering/blocks.go`, find the `dict` block's package list (it names `skilltag`, `classify`, `location`, `vocab`, `normalize`) and add `"answertopic"` in alphabetical position.

Run: `go test ./internal/platform/arch/layering/`
Expected: PASS. A failure here names the package that is in no block — if it does, the list you edited was the wrong one.

- [ ] **Step 6: Write the package AGENTS.md**

Create `internal/dict/answertopic/AGENTS.md`:

```markdown
# Answer topics

## Scope
`internal/dict/answertopic` — keys an employer's screening question by what it asks, so one
banked answer serves every phrasing of it.

## Always true
- **The dictionary is an accelerator, never the only route.** A question it does not know
  still keys, from the fold alone. This ordering is deliberate: `internal/api/atsapply`'s
  own `labelAnswerKeyFor` was a sole-route list, and its missing salary rule was invisible
  precisely because nothing reports the questions a list fails to match.
- **A question that folds to nothing is refused, never stored under an empty key.** Real
  forms produce empty labels — Greenhouse's hidden `required` proxy inputs carry no label
  at all — and a key derived from nothing can never be recalled.
- **Two different questions must never collapse.** `desired salary` and `current salary`
  are the worked example: merging them answers one with the other, in the candidate's name,
  to an employer. A test asserts they stay apart; keep it when adding dictionary entries.
- **Pure.** No model, no I/O, no user. A model proposing a topic is out of scope by design.
```

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/dict/answertopic/
go vet ./... && go test ./...
git add internal/dict/answertopic/ internal/platform/arch/layering/blocks.go
git commit -m "Key a screening question by what it asks, not how it is worded"
```

---

### Task 2: The table and its queries

**Files:**
- Create: `migrations/0155_screening_answer_bank.sql`
- Create: `internal/platform/db/queries/screening_answer_bank.sql`
- Modify: generated — `internal/platform/db/` (via `make sqlc`)

**Interfaces:**
- Consumes: nothing.
- Produces: `db.Queries` methods `UpsertScreeningAnswer(ctx, UpsertScreeningAnswerParams{UserID int64, Topic, Question, Answer, Provenance string}) error`, `ListScreeningAnswers(ctx, userID int64) ([]db.ScreeningAnswerBank, error)`, `DeleteScreeningAnswer(ctx, DeleteScreeningAnswerParams{ID int64, UserID int64}) (int64, error)`.

- [ ] **Step 1: Write the migration**

Create `migrations/0155_screening_answer_bank.sql`:

```sql
-- The candidate's own answers to employer screening questions, accumulating across
-- applications, so an answer given once serves every later posting that asks the same thing.
--
-- Distinct from screening_answers (0092), which holds SIX typed facts — desired salary as
-- (amount, currency, period), authorized countries as a validated array — and keeps them
-- typed because the product compares them (desired salary against candidate_survey's
-- current income, without conversion) and validates them. Those stay where they are. This
-- table takes the open-ended remainder: the questions employers author, which no fixed
-- column set can anticipate.
--
-- Keyed by TOPIC, not by the question's wording (internal/dict/answertopic). Keying on
-- wording is what fills a bank with near-duplicates and asks the candidate the same thing
-- once per employer.
--
-- question is kept verbatim beside it: a topic alone cannot show what was being asked, and
-- an answer sent to an employer in the candidate's name has to remain auditable against the
-- words they actually read.
--
-- provenance is text validated in Go rather than by a CHECK, matching how
-- screening_answers validates desired_salary_period and how the experience bank handles its
-- own provenance: one answer per repository to "where is an enum enforced", and adding a
-- vocabulary member stays a code change rather than a migration.
CREATE TABLE public.screening_answer_bank (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    topic      text        NOT NULL,
    question   text        NOT NULL,
    answer     text        NOT NULL,
    provenance text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- One answer per topic per candidate: the upsert key, and what makes a second phrasing
    -- of the same question find the existing answer instead of adding another.
    UNIQUE (user_id, topic)
);

-- Every read is "this candidate's answers", ordered for display. The UNIQUE above already
-- covers lookup by (user_id, topic), so this index serves only the list.
CREATE INDEX screening_answer_bank_user_updated_idx
    ON public.screening_answer_bank (user_id, updated_at DESC);
```

- [ ] **Step 2: Verify the migration lints**

Run: `pnpm check:sql`
Expected: it reports the file and passes. (`check-migrations: no migrations to check` means the file is not staged yet — `git add` it first.)

- [ ] **Step 3: Write the queries**

Create `internal/platform/db/queries/screening_answer_bank.sql`:

```sql
-- name: UpsertScreeningAnswer :exec
-- Records the candidate's answer to one screening question, replacing any earlier answer on
-- the same topic. The question text is refreshed too: the newest wording is the one they
-- most recently read and answered, and keeping a stale phrasing beside a fresh answer would
-- misdescribe what was agreed to.
--
-- provenance is overwritten on conflict rather than preserved: a candidate answering a
-- question themselves supersedes any earlier suggestion, and that is exactly the promotion
-- the send-gate depends on.
INSERT INTO screening_answer_bank (user_id, topic, question, answer, provenance)
VALUES (sqlc.arg(user_id), sqlc.arg(topic), sqlc.arg(question), sqlc.arg(answer), sqlc.arg(provenance))
ON CONFLICT (user_id, topic) DO UPDATE
SET question   = EXCLUDED.question,
    answer     = EXCLUDED.answer,
    provenance = EXCLUDED.provenance,
    updated_at = now();

-- name: ListScreeningAnswers :many
-- One candidate's whole bank, newest first — what the management surface lists and what the
-- profile assembler merges into the answer map.
SELECT id, user_id, topic, question, answer, provenance, created_at, updated_at
FROM screening_answer_bank
WHERE user_id = sqlc.arg(user_id)
ORDER BY updated_at DESC, id DESC;

-- name: DeleteScreeningAnswer :execrows
-- The owner removes one answer. Scoped by user_id as well as id, so a foreign id affects
-- zero rows and the handler renders 404 — never revealing to a probing caller which of the
-- two it was, the same posture GetAutoApplyQueueEntryForReview already takes.
--
-- Nothing else ever deletes from this table: the bank accumulates, and no reconciler prunes
-- it. Same rule as the experience bank, and for the same reason — a sweeper here would
-- silently discard answers the candidate expects to still hold.
DELETE FROM screening_answer_bank
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);
```

- [ ] **Step 4: Generate and verify**

Run: `make sqlc && go build ./...`
Expected: `internal/platform/db/screening_answer_bank.sql.go`, `models.go` and `querier.go` change; the build passes.

- [ ] **Step 5: Commit**

```bash
git add migrations/0155_screening_answer_bank.sql internal/platform/db/
git commit -m "Add the screening answer bank's table and queries"
```

---

### Task 3: The store and its provenance gate

**Files:**
- Create: `internal/candidate/answerbank/answerbank.go`
- Create: `internal/candidate/answerbank/repository.go`
- Create: `internal/candidate/answerbank/answerbank_test.go`
- Create: `internal/candidate/answerbank/AGENTS.md`
- Modify: `internal/platform/arch/layering/blocks.go`

**Interfaces:**
- Consumes: `answertopic.Of` (Task 1); `db.Queries` methods (Task 2).
- Produces:
  - `answerbank.Provenance` with `AuthorCandidate` and `AuthorAgent`.
  - `answerbank.Answer{ID int64, Topic, Question, Answer string, Provenance string, UpdatedAt time.Time}`.
  - `answerbank.NewStore(r Repository) *Store`.
  - `(*Store).Save(ctx, userID int64, question, answer string, by Provenance) error`.
  - `(*Store).List(ctx, userID int64) ([]Answer, error)`.
  - `(*Store).Delete(ctx, userID, id int64) error` returning `ErrNotFound` when nothing matched.
  - `(*Store).Sendable(ctx, userID int64) (map[string]string, error)` — topic → answer, candidate-authored only.
  - `answerbank.ErrEmptyAnswer`, `ErrUnkeyable`, `ErrTooLong`, `ErrNotFound`.

- [ ] **Step 1: Write the failing test**

Create `internal/candidate/answerbank/answerbank_test.go`:

```go
package answerbank

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRepo is an in-memory Repository. Keyed the way the table is — by (user, topic) — so
// the double cannot disagree with the UNIQUE constraint about what an upsert replaces.
type fakeRepo struct {
	rows map[int64]map[string]Answer
	next int64
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[int64]map[string]Answer{}} }

func (r *fakeRepo) Upsert(_ context.Context, userID int64, a Answer) error {
	if r.rows[userID] == nil {
		r.rows[userID] = map[string]Answer{}
	}
	existing, ok := r.rows[userID][a.Topic]
	if ok {
		a.ID = existing.ID
	} else {
		r.next++
		a.ID = r.next
	}
	r.rows[userID][a.Topic] = a
	return nil
}

func (r *fakeRepo) List(_ context.Context, userID int64) ([]Answer, error) {
	var out []Answer
	for _, a := range r.rows[userID] {
		out = append(out, a)
	}
	return out, nil
}

func (r *fakeRepo) Delete(_ context.Context, userID, id int64) (int64, error) {
	for topic, a := range r.rows[userID] {
		if a.ID == id {
			delete(r.rows[userID], topic)
			return 1, nil
		}
	}
	return 0, nil
}

func TestSave_TheSameQuestionPhrasedTwiceReplacesRatherThanDuplicates(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	if err := s.Save(ctx, 1, "What is your desired salary?", "5000 USD per year", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, 1, "Desired salary", "6000 USD per year", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.List(ctx, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("bank holds %d answers, want 1 — the two phrasings are one question: %+v", len(got), got)
	}
	if got[0].Answer != "6000 USD per year" {
		t.Errorf("answer = %q, want the newer one", got[0].Answer)
	}
}

// The gate the whole design rests on: an answer a model asserted is stored, listed, and
// never sent to an employer.
func TestSendable_OmitsAnAnswerTheCandidateDidNotGive(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	if err := s.Save(ctx, 1, "Which state do you currently reside in?", "Santa Catarina", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, 1, "How many years of Go do you have?", "8", AuthorAgent); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sendable, err := s.Sendable(ctx, 1)
	if err != nil {
		t.Fatalf("Sendable: %v", err)
	}
	if len(sendable) != 1 {
		t.Fatalf("sendable = %+v, want only the candidate's own answer", sendable)
	}
	for _, v := range sendable {
		if v != "Santa Catarina" {
			t.Errorf("sendable carries %q, want the candidate-authored answer", v)
		}
	}

	all, err := s.List(ctx, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List returned %d, want both — an agent's answer is stored and shown, just not sent", len(all))
	}
}

// A caller naming its own provenance is not evidence of who it is: anything unrecognised
// falls to the label that cannot be sent. Failing closed is the point.
func TestSave_AnUnrecognisedAuthorIsNotSendable(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	if err := s.Save(ctx, 1, "Do you have a work permit?", "Yes", Provenance("totally-the-candidate-honest")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sendable, err := s.Sendable(ctx, 1)
	if err != nil {
		t.Fatalf("Sendable: %v", err)
	}
	if len(sendable) != 0 {
		t.Errorf("sendable = %+v, want empty — an unrecognised author must fail closed", sendable)
	}
}

func TestSave_RefusesWhatCannotBeStoredHonestly(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	// An empty answer is not an answer. Storing one marks the question answered forever
	// with nothing to send.
	if err := s.Save(ctx, 1, "Which state?", "   ", AuthorCandidate); !errors.Is(err, ErrEmptyAnswer) {
		t.Errorf("Save(blank answer) = %v, want ErrEmptyAnswer", err)
	}
	// A question that cannot be keyed cannot be recalled.
	if err := s.Save(ctx, 1, "???", "Yes", AuthorCandidate); !errors.Is(err, ErrUnkeyable) {
		t.Errorf("Save(unkeyable question) = %v, want ErrUnkeyable", err)
	}
	// An essay is a cover letter, and there is a separate path for those.
	if err := s.Save(ctx, 1, "Why us?", strings.Repeat("a", MaxAnswerLen+1), AuthorCandidate); !errors.Is(err, ErrTooLong) {
		t.Errorf("Save(oversize answer) = %v, want ErrTooLong", err)
	}
}

func TestDelete_AForeignAnswerIsNotFound(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	if err := s.Save(ctx, 1, "Which state?", "SC", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}
	mine, _ := s.List(ctx, 1)

	if err := s.Delete(ctx, 2, mine[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete(another user's answer) = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, 1, mine[0].ID); err != nil {
		t.Errorf("Delete(own answer) = %v, want nil", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/candidate/answerbank/`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the store**

Create `internal/candidate/answerbank/answerbank.go`:

```go
// Package answerbank is the candidate's accumulating record of what they answer to
// employers' screening questions.
//
// It exists because the alternative does not scale: internal/api/atsapply matches a
// question to a stored fact through hand-written rules, and employers author questions
// faster than anyone writes rules. A production application parked on "What is your desired
// salary?" while the candidate's own figure sat in screening_answers, because no rule
// joined the two (freehire, 2026-09-08).
//
// It is a store, not a cache: it accumulates, and only its owner removes anything. No
// reconciler prunes it and no import replaces it — the same rule internal/candidate/
// experience states for the same reason, that a sweeper here would silently discard
// answers the candidate expects to still hold.
//
// It does NOT replace screening_answers, which holds six typed facts the product compares
// and validates (see migration 0155's own comment). This takes the open-ended remainder.
package answerbank

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/answertopic"
)

// MaxAnswerLen bounds one stored answer. Long enough for any screening answer a form
// actually asks for, short enough that a pasted essay is refused — that is a cover letter,
// and internal/candidate/coverletter is the path for those. Enforced here rather than only
// in the form because an API key reaches the write route too.
const MaxAnswerLen = 2000

// Provenance is who asserted an answer. Only the candidate's own may be sent to an
// employer — an answer on an application form is a claim made in their name.
type Provenance string

const (
	// AuthorCandidate is the person, answering from their own session or API key.
	AuthorCandidate Provenance = "candidate"
	// AuthorAgent is a model's reading, stored as a suggestion and never sent. Nothing
	// writes it yet; the value exists so the gate is built before there is anything to let
	// through — adding it later would mean adding the enforcement later too, to code that
	// had never needed it.
	AuthorAgent Provenance = "agent_inferred"
)

// sendable reports whether an answer with this provenance may go to an employer. Anything
// unrecognised — a new entry point that forgets to name itself — is not sendable. Failing
// closed is the point, and it is the same rule internal/candidate/experience applies to its
// own provenance.
func (p Provenance) sendable() bool { return p == AuthorCandidate }

// Answer is one banked answer.
type Answer struct {
	ID         int64
	Topic      string
	Question   string
	Answer     string
	Provenance string
	UpdatedAt  time.Time
}

var (
	// ErrEmptyAnswer refuses a blank answer. A blank is indistinguishable from an
	// unanswered question, and storing one would mark the question answered forever with
	// nothing to send.
	ErrEmptyAnswer = errors.New("answerbank: the answer is empty")
	// ErrUnkeyable refuses a question that folds to no topic — an empty or
	// punctuation-only label, both of which real forms produce. It could never be recalled.
	ErrUnkeyable = errors.New("answerbank: the question cannot be keyed")
	// ErrTooLong refuses an answer past MaxAnswerLen.
	ErrTooLong = errors.New("answerbank: the answer is too long")
	// ErrNotFound reports a delete that matched nothing — a missing id and another
	// candidate's id alike, so a probing caller learns nothing about which.
	ErrNotFound = errors.New("answerbank: no such answer")
)

// Repository is the storage this package needs. An interface so the store's own rules are
// testable without a database, matching how internal/candidate/experience separates the two.
type Repository interface {
	Upsert(ctx context.Context, userID int64, a Answer) error
	List(ctx context.Context, userID int64) ([]Answer, error)
	Delete(ctx context.Context, userID, id int64) (int64, error)
}

// Store is the bank's own rules over a Repository.
type Store struct{ repo Repository }

// NewStore builds a Store.
func NewStore(repo Repository) *Store { return &Store{repo: repo} }

// Save records an answer under the question's topic, replacing any earlier answer on the
// same topic.
//
// by is taken from the ENTRY POINT that received the request, never from a request body: a
// caller naming itself is not evidence of who it is. This is the same rule — and the same
// hazard — as experience.Author and cvedit.Actor.
func (s *Store) Save(ctx context.Context, userID int64, question, answer string, by Provenance) error {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return ErrEmptyAnswer
	}
	if len(trimmed) > MaxAnswerLen {
		return ErrTooLong
	}
	topic, ok := answertopic.Of(question)
	if !ok {
		return ErrUnkeyable
	}
	return s.repo.Upsert(ctx, userID, Answer{
		Topic:      topic,
		Question:   strings.TrimSpace(question),
		Answer:     trimmed,
		Provenance: string(by),
	})
}

// List returns the candidate's whole bank, whatever its provenance — the management surface
// shows an agent's suggestion so the candidate can confirm or correct it. What may be SENT
// is Sendable's question, not this one.
func (s *Store) List(ctx context.Context, userID int64) ([]Answer, error) {
	return s.repo.List(ctx, userID)
}

// Sendable returns topic → answer for the answers that may reach an employer.
func (s *Store) Sendable(ctx context.Context, userID int64) (map[string]string, error) {
	all, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(all))
	for _, a := range all {
		if Provenance(a.Provenance).sendable() {
			out[a.Topic] = a.Answer
		}
	}
	return out, nil
}

// Delete removes one of the candidate's own answers. Nothing else ever removes from this
// bank.
func (s *Store) Delete(ctx context.Context, userID, id int64) error {
	affected, err := s.repo.Delete(ctx, userID, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Write the repository**

Create `internal/candidate/answerbank/repository.go`:

```go
package answerbank

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Queries is the generated surface this repository needs — an interface rather than
// *db.Queries so a caller can substitute one, the same shape
// internal/candidate/experience's own repository takes.
type Queries interface {
	UpsertScreeningAnswer(ctx context.Context, arg db.UpsertScreeningAnswerParams) error
	ListScreeningAnswers(ctx context.Context, userID int64) ([]db.ScreeningAnswerBank, error)
	DeleteScreeningAnswer(ctx context.Context, arg db.DeleteScreeningAnswerParams) (int64, error)
}

// QueriesRepository adapts the generated queries to Repository.
type QueriesRepository struct{ q Queries }

// NewQueriesRepository builds a Repository over the generated queries.
func NewQueriesRepository(q Queries) *QueriesRepository { return &QueriesRepository{q: q} }

var _ Repository = (*QueriesRepository)(nil)

func (r *QueriesRepository) Upsert(ctx context.Context, userID int64, a Answer) error {
	return r.q.UpsertScreeningAnswer(ctx, db.UpsertScreeningAnswerParams{
		UserID: userID, Topic: a.Topic, Question: a.Question,
		Answer: a.Answer, Provenance: a.Provenance,
	})
}

func (r *QueriesRepository) List(ctx context.Context, userID int64) ([]Answer, error) {
	rows, err := r.q.ListScreeningAnswers(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Answer, 0, len(rows))
	for _, row := range rows {
		out = append(out, Answer{
			ID: row.ID, Topic: row.Topic, Question: row.Question,
			Answer: row.Answer, Provenance: row.Provenance,
			UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (r *QueriesRepository) Delete(ctx context.Context, userID, id int64) (int64, error) {
	return r.q.DeleteScreeningAnswer(ctx, db.DeleteScreeningAnswerParams{ID: id, UserID: userID})
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/candidate/answerbank/ -v`
Expected: PASS, all five tests.

- [ ] **Step 6: Register with the layering guard and write AGENTS.md**

In `internal/platform/arch/layering/blocks.go`, add `"answerbank"` to the `candidate` block's package list (the one naming `"cv", "cvedit", "cvmatch", "cvsection", "experience"`), in alphabetical position.

Create `internal/candidate/answerbank/AGENTS.md`:

```markdown
# The screening answer bank

## Scope
`internal/candidate/answerbank` — what the candidate answers to employers' screening
questions, accumulating across applications.

## Always true
- **The bank is a store, not a cache.** It accumulates; only its owner removes anything.
  No reconciler prunes it, no import replaces it — same rule as
  [the experience bank](../experience/AGENTS.md), and a sweeper here would silently discard
  answers the candidate expects to still hold.
- **Provenance is derived from the ENTRY POINT, never from the request body.** A caller
  naming itself is not evidence of who it is. Only `candidate` may be sent to an employer;
  anything unrecognised fails closed and is not sendable.
- **`List` shows everything, `Sendable` filters.** The management surface must show an
  agent's suggestion so the candidate can confirm it; the application path must not send
  one. Two methods, so the difference cannot be forgotten at a call site.
- **Keyed by topic, not by wording** (`internal/dict/answertopic`). Keying on wording is
  what fills a bank with near-duplicates and asks the same question once per employer.
- **It does not replace `screening_answers`.** Those six facts are typed because the product
  compares and validates them; this takes the open-ended remainder. See migration 0155.
```

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/candidate/answerbank/
go vet ./... && go test ./...
git add internal/candidate/answerbank/ internal/platform/arch/layering/blocks.go
git commit -m "Add the answer bank's store and its provenance gate"
```

---

### Task 4: Reaching the resolver

**Files:**
- Modify: `internal/api/candidateprofile/profile.go`
- Modify: `internal/api/atsapply/resolve.go`
- Modify: `internal/api/atsapply/resolve_test.go`
- Modify: `internal/api/handler/handler.go`
- Modify: `cmd/auto-apply/main.go`

**Interfaces:**
- Consumes: `(*answerbank.Store).Sendable` (Task 3); `answertopic.Of` (Task 1).
- Produces: answers map entries keyed `"topic:" + topic`; `candidateprofile.BankReader` interface with `Sendable(ctx context.Context, userID int64) (map[string]string, error)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/api/atsapply/resolve_test.go`:

```go
// A banked answer reaches a question no rule and no id could match — the bank's whole
// purpose. The key is the question's own topic, so the wording the employer used does not
// have to be the wording the candidate answered.
func TestResolve_AnsweredFromTheBankByTopic(t *testing.T) {
	answers := map[string]string{"topic:which state do you currently reside in": "Santa Catarina"}
	fields := []MergedField{{
		ID: "question_4005041004", Label: "Which state do you currently reside in?",
		Kind: "text", Required: true,
	}}

	plan := Resolve(fields, answers, false)

	if len(plan.Unmapped) != 0 {
		t.Fatalf("unmapped = %+v, want the question answered from the bank", plan.Unmapped)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Santa Catarina" {
		t.Fatalf("plan.Fields = %+v, want the banked answer", plan.Fields)
	}
}

// A typed fact still wins. It is validated and structured; the bank's copy is free text,
// and two sources answering one question must resolve the same way every time rather than
// by whichever was read first.
func TestResolve_ATypedFactOutranksABankedAnswer(t *testing.T) {
	answers := map[string]string{
		"desired_salary":                   "5000 USD per year",
		"topic:salary_expectation":         "whatever you think is fair",
	}
	fields := []MergedField{{ID: "question_1", Label: "What is your desired salary?", Kind: "text", Required: true}}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "5000 USD per year" {
		t.Fatalf("plan.Fields = %+v, want the typed fact to win", plan.Fields)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/atsapply/ -run "TestResolve_AnsweredFromTheBank|TestResolve_ATypedFact" -v`
Expected: the first FAILs (the question lands in `Unmapped`); the second PASSes already, because nothing reads the bank yet. Both must pass by Step 4.

- [ ] **Step 3: Add the fallback lookup**

In `internal/api/atsapply/resolve.go`, add the import `"github.com/strelov1/freehire/internal/dict/answertopic"`, then add this function beside `matchLabelAnswerKey`:

```go
// bankAnswerKeyPrefix namespaces the banked answers inside the same map the deterministic
// facts use. One map rather than two arguments threaded through every call site: a banked
// answer IS an answer, and the resolver has no reason to know which source stated it.
//
// The prefix keeps the two from colliding — a topic is a folded question, which can be any
// text at all, including the exact string "email".
const bankAnswerKeyPrefix = "topic:"

// matchBankAnswerKey returns the answers-map key a field's label is banked under, if the
// label can be keyed at all. Checked AFTER the id and label rules, never before: those are
// typed, validated facts, and the bank's copy is free text — two sources answering one
// question have to resolve the same way every time rather than by read order.
func matchBankAnswerKey(label string) (string, bool) {
	topic, ok := answertopic.Of(label)
	if !ok {
		return "", false
	}
	return bankAnswerKeyPrefix + topic, true
}
```

Then, in `resolveOne`, find where the label rule is consulted (`matchLabelAnswerKey`) and add the bank as the next fallback. The existing shape is:

```go
	key, ok := answerKeyFor[f.ID]
	if !ok {
		key, ok = matchLabelAnswerKey(f.Label)
	}
```

Extend it to:

```go
	key, ok := answerKeyFor[f.ID]
	if !ok {
		key, ok = matchLabelAnswerKey(f.Label)
	}
	if !ok {
		// The bank: an answer the candidate gave to this same question on an earlier
		// application. Last, so a typed fact always wins — see matchBankAnswerKey.
		key, ok = matchBankAnswerKey(f.Label)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/atsapply/ -v`
Expected: PASS, the whole package.

- [ ] **Step 5: Merge banked answers into the assembled profile**

In `internal/api/candidateprofile/profile.go`:

Add the reader interface beside `ScreeningAnswersReader`:

```go
// BankReader supplies the candidate's own banked screening answers, keyed by topic. Only
// answers they themselves gave — internal/candidate/answerbank.Store.Sendable is what
// enforces that, not this interface.
type BankReader interface {
	Sendable(ctx context.Context, userID int64) (map[string]string, error)
}
```

Add the field to `Assembler` and the parameter to `NewAssembler` (both nil-able, matching `screeningAnswers`):

```go
	// bank is nil-able: "no answer bank configured" degrades to no banked answers, the
	// same way an unconfigured screening-answers reader degrades those fields to empty.
	bank BankReader
```

Add to `Profile`:

```go
	// BankAnswers is topic → answer for the questions this candidate has answered on
	// earlier applications. Not a fixed field like the ones above, because the questions
	// employers author are not a fixed set — that is the whole reason the bank exists.
	BankAnswers map[string]string `json:"-"`
```

In `Fields()`, merge them after building the fixed map, under the prefix `resolve.go` looks
them up by:

```go
func (p Profile) Fields() map[string]string {
	fields := map[string]string{
		// ... unchanged existing entries ...
	}
	// Banked answers ride in the same map under a prefix, so a caller that resolves an
	// answer never has to know which source stated it. The prefix must match
	// internal/api/atsapply's own bankAnswerKeyPrefix — a topic is a folded question and
	// could otherwise collide with a fixed key.
	for topic, answer := range p.BankAnswers {
		fields["topic:"+topic] = answer
	}
	return fields
}
```

In `Assemble`, after `applyScreeningFields`:

```go
	if a.bank != nil {
		banked, err := a.bank.Sendable(ctx, userID)
		if err != nil {
			return Profile{}, err
		}
		profile.BankAnswers = banked
	}
```

- [ ] **Step 6: Wire the store at both call sites**

In `internal/api/handler/handler.go`, find the existing `candidateprofile.NewAssembler(...)` call and pass a store built from the same `queries`:

```go
	answerBank := answerbank.NewStore(answerbank.NewQueriesRepository(queries))
```

...then add `answerBank` as `NewAssembler`'s new final argument.

In `cmd/auto-apply/main.go`, find its own `candidateprofile.NewAssembler(...)` call and do the same. The worker is the path that actually fills forms — an assembler without the bank here would leave the feature working in the UI and silently absent where it matters.

- [ ] **Step 7: Verify the whole build and suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS. A compile error at either `NewAssembler` call site means one was missed.

Run: `go vet -tags=integration ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/ cmd/
git add internal/ cmd/
git commit -m "Let a banked answer reach the form resolver"
```

---

### Task 5: The three routes

**Files:**
- Create: `internal/api/handler/me_answer_bank.go`
- Create: `internal/api/handler/me_answer_bank_integration_test.go`
- Modify: `internal/api/handler/handler.go` (register the routes)

**Interfaces:**
- Consumes: `*answerbank.Store` (Task 3).
- Produces: `GET /api/v1/me/answer-bank`, `PUT /api/v1/me/answer-bank`, `DELETE /api/v1/me/answer-bank/:id`.

- [ ] **Step 1: Write the failing test**

Create `internal/api/handler/me_answer_bank_integration_test.go`:

```go
//go:build integration

// Integration tests for the answer bank's own routes. Run with:
//   go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

// Saving an answer, then reading it back, is the whole loop the review screen performs.
func TestAnswerBank_SavedAnswerIsListedBack(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAnswerBankApp(pool, iss)
	_, cookie := answerBankUser(t, pool, iss, "bank@example.test")

	save := answerBankRequest(t, app, fiber.MethodPut, "/api/v1/me/answer-bank", cookie,
		map[string]string{"question": "Which state do you currently reside in?", "answer": "Santa Catarina"})
	defer save.Body.Close()
	if save.StatusCode != fiber.StatusOK {
		t.Fatalf("save status = %d, want 200", save.StatusCode)
	}

	list := answerBankRequest(t, app, fiber.MethodGet, "/api/v1/me/answer-bank", cookie, nil)
	defer list.Body.Close()
	if list.StatusCode != fiber.StatusOK {
		t.Fatalf("list status = %d, want 200", list.StatusCode)
	}
	var out struct {
		Data []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
		} `json:"data"`
	}
	decodeJSON(t, list, &out)
	if len(out.Data) != 1 || out.Data[0].Answer != "Santa Catarina" {
		t.Fatalf("data = %+v, want the saved answer", out.Data)
	}
}

// The same question worded differently updates the answer rather than adding a second one.
func TestAnswerBank_ARephrasedQuestionUpdatesRatherThanDuplicates(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAnswerBankApp(pool, iss)
	_, cookie := answerBankUser(t, pool, iss, "rephrase@example.test")

	for _, body := range []map[string]string{
		{"question": "What is your desired salary?", "answer": "5000 USD per year"},
		{"question": "Salary expectations", "answer": "6000 USD per year"},
	} {
		resp := answerBankRequest(t, app, fiber.MethodPut, "/api/v1/me/answer-bank", cookie, body)
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("save status = %d, want 200", resp.StatusCode)
		}
	}

	list := answerBankRequest(t, app, fiber.MethodGet, "/api/v1/me/answer-bank", cookie, nil)
	defer list.Body.Close()
	var out struct {
		Data []struct {
			Answer string `json:"answer"`
		} `json:"data"`
	}
	decodeJSON(t, list, &out)
	if len(out.Data) != 1 {
		t.Fatalf("bank holds %d answers, want 1 — the two phrasings are one question", len(out.Data))
	}
	if out.Data[0].Answer != "6000 USD per year" {
		t.Errorf("answer = %q, want the newer one", out.Data[0].Answer)
	}
}

// Another candidate's answer is reported missing, never forbidden — the same posture every
// other owned resource here takes, so a probing caller learns nothing.
func TestAnswerBank_AForeignAnswerIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAnswerBankApp(pool, iss)
	_, ownerCookie := answerBankUser(t, pool, iss, "owner@example.test")
	_, otherCookie := answerBankUser(t, pool, iss, "other@example.test")

	save := answerBankRequest(t, app, fiber.MethodPut, "/api/v1/me/answer-bank", ownerCookie,
		map[string]string{"question": "Which state?", "answer": "SC"})
	save.Body.Close()

	list := answerBankRequest(t, app, fiber.MethodGet, "/api/v1/me/answer-bank", ownerCookie, nil)
	defer list.Body.Close()
	var out struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, list, &out)

	del := answerBankRequest(t, app, fiber.MethodDelete,
		"/api/v1/me/answer-bank/"+itoa(out.Data[0].ID), otherCookie, nil)
	defer del.Body.Close()
	if del.StatusCode != http.StatusNotFound {
		t.Errorf("delete status = %d, want 404", del.StatusCode)
	}
}
```

**Harness note for the implementer:** `startPostgres`, `decodeJSON` and the cookie helpers
already exist in this package's other integration tests. Model `newAnswerBankApp`,
`answerBankUser`, `answerBankRequest` and `itoa` on
`internal/api/handler/auto_apply_tailor_integration_test.go`'s own
`newAutoApplyTailorApp` / `autoApplyTailorUser` / `autoApplyRequest`, which do exactly this
for another `/me` route. Do not invent a second harness shape.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags=integration ./internal/api/handler/ -run TestAnswerBank`
Expected: FAIL to build — `newAnswerBankApp` undefined.

- [ ] **Step 3: Write the handlers**

Create `internal/api/handler/me_answer_bank.go`:

```go
package handler

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/answerbank"
)

// answerBankHandlers serves the candidate's own bank of screening answers.
type answerBankHandlers struct{ bank *answerbank.Store }

// RegisterAnswerBankRoutes mounts the bank's three routes. mw.key, matching every other
// /me route that a client other than the web app may reasonably call: the CLI
// (strelov1/freehire-cli) reaches these with an API key.
func (h *answerBankHandlers) RegisterAnswerBankRoutes(api fiber.Router, mw middlewares) {
	api.Get("/me/answer-bank", mw.key, h.ListAnswers)
	api.Put("/me/answer-bank", mw.key, h.SaveAnswer)
	api.Delete("/me/answer-bank/:id", mw.key, h.DeleteAnswer)
}

// bankedAnswerResponse is one answer on the wire. provenance rides along so a surface can
// show which answers are the candidate's own — the store's List returns every provenance
// deliberately.
type bankedAnswerResponse struct {
	ID         int64  `json:"id"`
	Topic      string `json:"topic"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
	Provenance string `json:"provenance"`
	UpdatedAt  string `json:"updated_at"`
}

// ListAnswers returns the caller's whole bank, newest first.
func (h *answerBankHandlers) ListAnswers(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	answers, err := h.bank.List(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]bankedAnswerResponse, 0, len(answers))
	for _, a := range answers {
		out = append(out, bankedAnswerResponse{
			ID: a.ID, Topic: a.Topic, Question: a.Question, Answer: a.Answer,
			Provenance: a.Provenance, UpdatedAt: a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return c.JSON(fiber.Map{"data": out})
}

// saveAnswerRequest is what the review screen and the CLI both send.
type saveAnswerRequest struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// SaveAnswer records the caller's answer to one question.
//
// The provenance is AuthorCandidate because of WHERE this ran — a request the candidate
// themselves authenticated — never because a body said so. There is no field for it in
// saveAnswerRequest, and that absence is the enforcement.
func (h *answerBankHandlers) SaveAnswer(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in saveAnswerRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	err = h.bank.Save(c.Context(), userID, in.Question, in.Answer, answerbank.AuthorCandidate)
	switch {
	case errors.Is(err, answerbank.ErrEmptyAnswer):
		return fiber.NewError(fiber.StatusBadRequest, "the answer is empty")
	case errors.Is(err, answerbank.ErrUnkeyable):
		return fiber.NewError(fiber.StatusBadRequest, "this question cannot be saved — it has no readable text")
	case errors.Is(err, answerbank.ErrTooLong):
		return fiber.NewError(fiber.StatusBadRequest, "this answer is too long for a screening question")
	case err != nil:
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"saved": true}})
}

// DeleteAnswer removes one of the caller's own answers. Nothing else ever removes from the
// bank.
func (h *answerBankHandlers) DeleteAnswer(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid answer id")
	}
	if err := h.bank.Delete(c.Context(), userID, id); err != nil {
		if errors.Is(err, answerbank.ErrNotFound) {
			// 404, never 403: a foreign id and a missing one look alike, so a probing
			// caller learns nothing about what another candidate holds.
			return fiber.NewError(fiber.StatusNotFound, "no such answer")
		}
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"deleted": true}})
}
```

**Note:** `middlewares` and `requireUserID` are this package's existing names — check the
exact spelling in `internal/api/handler/handler.go` and match it rather than assuming.

- [ ] **Step 4: Register the routes**

In `internal/api/handler/handler.go`, beside the other `/me` route registrations, construct
the handlers over the store built in Task 4 Step 6 and call
`answerBankH.RegisterAnswerBankRoutes(api, mw)`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -tags=integration ./internal/api/handler/ -run TestAnswerBank -v`
Expected: PASS, all three.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/api/handler/
go vet -tags=integration ./... && go test ./...
git add internal/api/handler/
git commit -m "Serve the answer bank over its own three routes"
```

---

### Task 6: Answering from the review screen

**Files:**
- Create: `web/src/lib/answerBank.ts`
- Create: `web/src/lib/answerBank.test.ts`
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/lib/components/JobDrawer.svelte`

**Interfaces:**
- Consumes: the routes from Task 5.
- Produces: `answerableQuestions(pending)` in `answerBank.ts`; `api.saveBankedAnswer(question, answer)` in `api.ts`.

- [ ] **Step 1: Write the failing test**

Create `web/src/lib/answerBank.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { answerableQuestions } from './answerBank';

describe('answerableQuestions', () => {
  it('offers an input for a question that has readable text', () => {
    const pending = [
      { label: 'What is your desired salary?', will_draft_at_submission: false },
      { label: 'Which state do you currently reside in?', will_draft_at_submission: false }
    ];
    expect(answerableQuestions(pending).map((q) => q.label)).toEqual([
      'What is your desired salary?',
      'Which state do you currently reside in?'
    ]);
  });

  // A question the model will fill needs no input from the candidate — offering one asks
  // them to do work that is already handled.
  it('skips a question that will be drafted at submission', () => {
    const pending = [{ label: 'Why do you want to work here?', will_draft_at_submission: true }];
    expect(answerableQuestions(pending)).toEqual([]);
  });

  // A labelless entry cannot be answered: there is nothing to show the candidate, and the
  // server refuses to key it. Rendering a blank input invites an answer to a question
  // nobody can see.
  it('skips a question with no readable text', () => {
    const pending = [
      { label: '', will_draft_at_submission: false },
      { label: '   ', will_draft_at_submission: false }
    ];
    expect(answerableQuestions(pending)).toEqual([]);
  });

  it('is empty for no pending questions at all', () => {
    expect(answerableQuestions(undefined)).toEqual([]);
    expect(answerableQuestions([])).toEqual([]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/lib/answerBank.test.ts`
Expected: FAIL — cannot resolve `./answerBank`.

(If the whole file fails to transform in a fresh worktree, run `pnpm exec svelte-kit sync`
first — a fresh checkout has no generated types and every test file fails to load.)

- [ ] **Step 3: Write the helper**

Create `web/src/lib/answerBank.ts`:

```ts
import type { AutoApplyPreviewPending } from '$lib/types';

/** The pending questions worth offering the candidate an input for.
 *
 *  Kept out of JobDrawer.svelte so it unit-tests without mounting Svelte, the same
 *  convention autoApplyReview.ts and autoApplyButton.ts already follow.
 *
 *  Two are skipped. One that will be drafted at submission needs nothing from the
 *  candidate — asking anyway is work we already handle. One with no readable label cannot
 *  be answered at all: there is nothing to show, and the server refuses to key a question
 *  that folds to nothing, so an input there would collect an answer that could never be
 *  saved. */
export function answerableQuestions(
  pending: AutoApplyPreviewPending[] | undefined | null
): AutoApplyPreviewPending[] {
  if (!pending) return [];
  return pending.filter((p) => !p.will_draft_at_submission && p.label.trim() !== '');
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm vitest run src/lib/answerBank.test.ts`
Expected: PASS, all four.

- [ ] **Step 5: Add the client call and its type**

In `web/src/lib/types.ts`, beside the other auto-apply types:

```ts
/** One answer the candidate has banked for a screening question. */
export interface BankedAnswer {
  id: number;
  topic: string;
  question: string;
  answer: string;
  provenance: string;
  updated_at: string;
}
```

In `web/src/lib/api.ts`, beside `reviewAutoApply`, following that method's exact shape for
headers and error handling:

```ts
  saveBankedAnswer(question: string, answer: string): Promise<void>,
  listBankedAnswers(): Promise<BankedAnswer[]>,
  deleteBankedAnswer(id: number): Promise<void>,
```

- [ ] **Step 6: Render the inputs**

In `web/src/lib/components/JobDrawer.svelte`, inside the `pending_review` banner, replace
the read-only pending list:

```svelte
              {#if autoApply?.resolved_preview?.pending?.length}
                <ul class="flex flex-col gap-0.5 text-xs text-muted-foreground">
                  {#each autoApply.resolved_preview.pending as p (p.label)}
                    <li>{p.label} — {p.will_draft_at_submission ? 'will be filled in automatically' : 'no known answer yet'}</li>
                  {/each}
                </ul>
              {/if}
```

...with a list that offers an input for the ones the candidate can answer:

```svelte
              {#if autoApply?.resolved_preview?.pending?.length}
                <ul class="flex flex-col gap-2 text-xs text-muted-foreground">
                  {#each autoApply.resolved_preview.pending as p (p.label)}
                    {#if p.will_draft_at_submission}
                      <li>{p.label} — will be filled in automatically</li>
                    {:else if p.label.trim() !== ''}
                      <li class="flex flex-col gap-1">
                        <label class="text-foreground" for={`bank-${p.label}`}>{p.label}</label>
                        <div class="flex gap-2">
                          <input
                            id={`bank-${p.label}`}
                            class="min-w-0 flex-1 rounded-md border border-border bg-background px-2 py-1"
                            bind:value={bankDrafts[p.label]}
                            placeholder="Your answer — saved for next time too"
                          />
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={bankSaving === p.label || !bankDrafts[p.label]?.trim()}
                            onclick={() => saveBankedAnswer(p.label)}
                          >
                            Save
                          </Button>
                        </div>
                      </li>
                    {/if}
                  {/each}
                </ul>
                {#if bankError}
                  <p class="text-xs text-destructive">{bankError}</p>
                {/if}
              {/if}
```

Add the state and handler to the component's `<script>`, beside `decideAutoApply`:

```ts
  // One draft per pending question, keyed by its label — the same key the {#each} uses, so
  // an input and its draft cannot drift apart.
  let bankDrafts = $state<Record<string, string>>({});
  let bankSaving = $state<string | null>(null);
  let bankError = $state<string | null>(null);

  async function saveBankedAnswer(question: string) {
    const answer = bankDrafts[question]?.trim();
    if (!answer || bankSaving) return;
    bankSaving = question;
    bankError = null;
    try {
      await api.saveBankedAnswer(question, answer);
      // Cleared rather than left filled: the answer now lives in the bank, and a filled
      // input beside a saved answer reads as unsaved work.
      bankDrafts = { ...bankDrafts, [question]: '' };
    } catch (e) {
      bankError = errorMessage(e, 'Could not save your answer.');
    } finally {
      bankSaving = null;
    }
  }
```

- [ ] **Step 7: Verify the front end**

Run: `cd web && pnpm vitest run && pnpm lint`
Expected: every test passes; lint reports no NEW issue in the files you touched (the repo
carries a pre-existing warning backlog — only new-from-main fails).

Run: `cd web && pnpm check 2>&1 | grep -E "ERROR" | grep -iE "answerBank|JobDrawer|types.ts"`
Expected: no output. (The project as a whole has ~41 pre-existing errors; only yours matter.)

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/answerBank.ts web/src/lib/answerBank.test.ts web/src/lib/types.ts web/src/lib/api.ts web/src/lib/components/JobDrawer.svelte
git commit -m "Let the candidate answer a blocking question from the review screen"
```

---

### Task 7: Prove the loop end to end

**Files:**
- Modify: `internal/api/atsapply/resolve_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing new — this task adds only the test that the feature actually works.

- [ ] **Step 1: Write the failing test**

Append to `internal/api/atsapply/resolve_test.go`:

```go
// The feature, asserted as one story: a required question parks, the candidate answers it,
// and the next resolve fills it — even though the second employer words it differently.
//
// This is the test that would have caught the whole class of bug this feature exists for.
// Everything else here checks a piece.
func TestResolve_AnAnsweredQuestionStopsBlockingLaterApplications(t *testing.T) {
	firstEmployer := []MergedField{{
		ID: "question_4005041004", Label: "Which state do you currently reside in?",
		Kind: "text", Required: true,
	}}
	noAnswersYet := map[string]string{}

	before := Resolve(firstEmployer, noAnswersYet, false)
	if len(before.Unmapped) != 1 {
		t.Fatalf("unmapped = %+v, want the question to park before it is answered", before.Unmapped)
	}
	if before.FullyResolved() {
		t.Fatal("FullyResolved() is true with a required question unanswered")
	}

	// The candidate answers it. answertopic.Of is what the server applies on save; the key
	// here is what that produces.
	banked := map[string]string{"topic:which state do you currently reside in": "Santa Catarina"}

	secondEmployer := []MergedField{{
		ID: "question_99887766", Label: "  Which state do you currently reside in?  ",
		Kind: "text", Required: true,
	}}

	after := Resolve(secondEmployer, banked, false)
	if !after.FullyResolved() {
		t.Fatalf("unmapped = %+v, want a different employer's phrasing answered from the bank", after.Unmapped)
	}
	if after.Fields[0].Value != "Santa Catarina" {
		t.Errorf("value = %q, want the banked answer", after.Fields[0].Value)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./internal/api/atsapply/ -run TestResolve_AnAnsweredQuestionStopsBlocking -v`
Expected: PASS — everything it needs was built in Tasks 1-5. If it fails, the failure names
which piece is not connected.

- [ ] **Step 3: Run the full suite**

Run: `go build ./... && go vet ./... && go test ./... && go vet -tags=integration ./...`
Expected: all clean.

- [ ] **Step 4: Commit and open the PR**

```bash
git add internal/api/atsapply/resolve_test.go
git commit -m "Assert the answer bank's whole loop in one test"
git push -u origin <branch>
gh pr create --title "The screening answer bank" --body "..."
```

The PR body should carry: the production measurement that motivated it (entry 3 parking on
a question the candidate had already answered), the two-layer scope decision and the row
counts behind it, and the provenance rule. The spec at
`docs/superpowers/specs/2026-09-09-screening-answer-bank-design.md` holds all three.

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Two layers, one interface | 4 |
| `screening_answer_bank` table | 2 |
| Topic: dictionary then fold | 1 |
| Provenance, derived from entry point | 3, 5 |
| Flow: park → answer → next resolve fills | 6, 7 |
| Surfaces: GET/PUT/DELETE, cookie or API key | 5 |
| CLI needs no server work beyond the routes | 5 (noted) |
| Empty answer is a refusal | 3 |
| Unkeyable question refused | 1, 3 |
| 2000-character bound, enforced in the service | 3 |
| Bank never overrides a typed fact | 4 |
| Package placement + layering registration | 1, 3 |
| Not in scope: agent writes, backfill, auto re-run | absent by construction |

**Type consistency:** `answertopic.Of` returns `(string, bool)` in Tasks 1, 3 and 4.
`Provenance` is a string type with `AuthorCandidate`/`AuthorAgent` in Tasks 3 and 5.
`bankAnswerKeyPrefix` is `"topic:"` in Task 4's resolver and the same literal in Task 4's
`Fields()` — they are in different packages and cannot share a constant without an import
that would invert the layering, so both carry a comment naming the other.

**Placeholders:** none. Every step carries the code it asks for. The two "match the existing
name" notes (Task 5's `middlewares`/`requireUserID`, Task 6's `api.ts` shape) point at
files the implementer will have open, rather than leaving a decision unmade.
