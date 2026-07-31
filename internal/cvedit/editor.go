package cvedit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/cv"
)

// Actor is who made a change. It is decided by the entry point that received the request and
// is never read from the request body: a caller naming itself is not evidence of who it is.
type Actor string

const (
	ActorCandidate Actor = "candidate"
	ActorAgent     Actor = "agent"
	ActorSystem    Actor = "system"
)

// Origin is the entry point a change arrived through. It separates two changes by the same
// actor that should not be folded together — a template pick and a burst of typing are both
// the candidate, and coalescing them would describe neither.
type Origin string

const (
	OriginEditor      Origin = "editor"
	OriginTailorAgent Origin = "tailor_agent"
	OriginCLI         Origin = "cli"
	OriginTemplate    Origin = "template"
	OriginImport      Origin = "import"
)

// Revision is one recorded change: what it did, what would undo it, who made it, and the
// document version it was computed against.
type Revision struct {
	ID      uuid.UUID
	CVID    uuid.UUID
	Actor   Actor
	Origin  Origin
	BatchID uuid.UUID // zero when the change stands alone
	Title   string
	Note    string
	Ops     []Op
	Inverse []Op
	// BaseVersion is the document's updated_at before the change. It is journalling, not
	// locking: the row is locked for the duration of a commit, and an operation addressing a
	// path that has since disappeared is refused when applied.
	BaseVersion time.Time
	RevertsID   uuid.UUID // the revision this one undoes, when it is an undo
	RevertedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Reverted reports whether this revision has already been undone, which is what greys out
// its control in the feed.
func (r Revision) Reverted() bool { return r.RevertedAt != nil }

// Change is a commit as a caller states it.
type Change struct {
	Actor   Actor
	Origin  Origin
	BatchID uuid.UUID
	Note    string
	Ops     []Op
}

// Repository is the owner-scoped persistence Editor writes through. Edit runs its callback
// against a locked CV row inside one transaction: the document and the revision describing
// it are written together, or neither is.
type Repository interface {
	Edit(ctx context.Context, cvID uuid.UUID, userID int64, fn func(context.Context, Tx) error) error
	List(ctx context.Context, cvID uuid.UUID, userID int64, limit int32) ([]Revision, error)
	Get(ctx context.Context, id uuid.UUID, userID int64) (Revision, error)
}

// Tx is the transactional view of one CV: everything a commit does, it does through here.
type Tx interface {
	State(ctx context.Context) (State, time.Time, error)
	Save(ctx context.Context, state State) (cv.Meta, error)
	Newest(ctx context.Context) (Revision, bool, error)
	Revision(ctx context.Context, id uuid.UUID) (Revision, bool, error)
	Insert(ctx context.Context, rev Revision) (Revision, error)
	Amend(ctx context.Context, id uuid.UUID, ops []Op, title string) (Revision, error)
	MarkReverted(ctx context.Context, id uuid.UUID) (bool, error)
	InBatch(ctx context.Context, batchID uuid.UUID) ([]Revision, error)
	Trim(ctx context.Context, keep int32) error
}

// ErrNothingToUndo is returned when an undo names a revision that has already been undone,
// or a batch that never edited this CV.
var ErrNothingToUndo = errors.New("cvedit: nothing to undo")

// ErrCannotUndo is returned when a revision's inverse no longer applies — the place it would
// restore is gone. Handlers render it as a conflict, because it is a fact about the document
// now, not a malformed request.
var ErrCannotUndo = errors.New("cvedit: this edit can no longer be undone")

// feedLimit is how many revisions one CV keeps. The log is an aid to the candidate's current
// work rather than an archive, and each row carries two operation documents.
const feedLimit = 100

// coalesceWindow is how long a revision stays open to absorbing a follow-on edit of the same
// place. Autosave fires 800ms after a keystroke, so without this a typed paragraph would file
// a revision per burst and undo would step back one debounce interval at a time.
const coalesceWindow = time.Minute

// Editor is the only thing that writes a stored CV. Every entry point — the editor's
// autosave, the template picker, the CLI, an agent tool, seeding a tailored copy — commits
// through here, so no change can happen without a revision recording it.
type Editor struct {
	repo   Repository
	policy Policy
	gate   EvidenceGate
	now    func() time.Time
}

// NewEditor builds the editor over its repository. The gate may be nil, which means no
// evidence is required — the CLI and the editor commit as the candidate, who is asserting
// things about themselves.
func NewEditor(repo Repository, gate EvidenceGate) *Editor {
	return &Editor{repo: repo, policy: DefaultPolicy(), gate: gate, now: time.Now}
}

// WithEvidenceGate attaches the bank the agent's claims are checked against. Assigned after
// construction because the bank is wired later than the CV handlers — the same shape the
// other late-bound dependencies here use. Without it the agent can write unevidenced claims,
// so it is a wiring mistake rather than a configuration choice.
func (e *Editor) WithEvidenceGate(gate EvidenceGate) { e.gate = gate }

// Commit applies a batch and records it. The document and its revision are written in one
// transaction against a locked row, so a reader never sees one without the other.
func (e *Editor) Commit(ctx context.Context, cvID uuid.UUID, userID int64, ch Change) (cv.Meta, Revision, error) {
	var (
		meta cv.Meta
		rev  Revision
	)
	err := e.repo.Edit(ctx, cvID, userID, func(ctx context.Context, tx Tx) error {
		var err error
		meta, rev, err = e.commit(ctx, tx, cvID, userID, ch)
		return err
	})
	return meta, rev, err
}

func (e *Editor) commit(ctx context.Context, tx Tx, cvID uuid.UUID, userID int64, ch Change) (cv.Meta, Revision, error) {
	if len(ch.Ops) == 0 {
		return cv.Meta{}, Revision{}, fmt.Errorf("%w: a change with no operations", ErrInvalidOp)
	}
	if err := e.policy.Allows(ch.Actor, ch.Ops); err != nil {
		return cv.Meta{}, Revision{}, err
	}
	if err := e.requireEvidence(ctx, userID, ch); err != nil {
		return cv.Meta{}, Revision{}, err
	}

	before, baseVersion, err := tx.State(ctx)
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}
	after, inverse, err := Apply(before, ch.Ops)
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}

	// Sanitize after applying and before storing, exactly as the whole-document path always
	// has: a change states what was asked for, and the sanitizer decides what is kept.
	after.Document.Sanitize()

	// An operation that changes nothing files nothing. Picking the template already in use
	// is a click the gallery allows, and "switched to the Sidebar template" under a CV that
	// was already on Sidebar is a lie the feed would keep forever.
	if Equal(before, after) {
		return metaOf(cvID, before), Revision{}, nil
	}

	meta, err := tx.Save(ctx, after)
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}

	rev, err := e.record(ctx, tx, cvID, userID, ch, inverse, baseVersion, before, after)
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}
	if err := tx.Trim(ctx, feedLimit); err != nil {
		return cv.Meta{}, Revision{}, err
	}
	return meta, rev, nil
}

// record files the change, folding it into the newest revision when it continues that one.
func (e *Editor) record(ctx context.Context, tx Tx, cvID uuid.UUID, userID int64, ch Change,
	inverse []Op, baseVersion time.Time, before, after State) (Revision, error) {

	title := Describe(ch.Ops, before, after)

	newest, ok, err := tx.Newest(ctx)
	if err != nil {
		return Revision{}, err
	}
	if ok && e.continues(newest, ch) {
		// The operations are replaced, not accumulated: each save carries the place's
		// current value, so the newest batch already describes the whole burst. Appending
		// would also make the revision stop matching the next save's shape, which is how
		// this was wrong the first time.
		//
		// The inverse is deliberately not touched: it still leads back to the state before
		// the first of the coalesced edits, which is what undo has to mean for typed text.
		return tx.Amend(ctx, newest.ID, ch.Ops, title)
	}

	return tx.Insert(ctx, Revision{
		CVID:        cvID,
		Actor:       ch.Actor,
		Origin:      ch.Origin,
		BatchID:     ch.BatchID,
		Title:       title,
		Note:        ch.Note,
		Ops:         ch.Ops,
		Inverse:     inverse,
		BaseVersion: baseVersion,
	})
}

// continues reports whether a change carries on the newest revision rather than starting a
// new one: the same hand, through the same door, at the same places, within the window.
func (e *Editor) continues(newest Revision, ch Change) bool {
	if newest.Actor != ch.Actor || newest.Origin != ch.Origin || newest.Reverted() {
		return false
	}
	if newest.RevertsID != uuid.Nil {
		return false // an undo is a landmark; folding an edit into it would hide both
	}
	if e.now().Sub(newest.UpdatedAt) > coalesceWindow {
		return false
	}
	return samePaths(newest.Ops, ch.Ops)
}

func samePaths(a, b []Op) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Kind != b[i].Kind {
			return false
		}
	}
	return true
}

// CommitDocument records a whole-document save. The differ derives the operations, so from
// here on a save is indistinguishable from an agent's batch — whole-document PUT is an input
// format, not a second way to write the document.
//
// A save that changes nothing commits nothing and returns the CV as it stands: autosave
// fires on a timer, and a revision per timer tick would bury the feed.
func (e *Editor) CommitDocument(ctx context.Context, cvID uuid.UUID, userID int64,
	actor Actor, origin Origin, next State) (cv.Meta, Revision, error) {

	var (
		meta cv.Meta
		rev  Revision
	)
	err := e.repo.Edit(ctx, cvID, userID, func(ctx context.Context, tx Tx) error {
		before, _, err := tx.State(ctx)
		if err != nil {
			return err
		}
		// Sanitize the incoming state first, so the diff is against what would be stored
		// rather than against what was sent — otherwise a value the sanitizer clamps would
		// show up as an edit on every save.
		next.Document.Sanitize()

		ops := Diff(before, next)
		if len(ops) == 0 {
			meta = metaOf(cvID, before)
			return nil
		}
		meta, rev, err = e.commit(ctx, tx, cvID, userID, Change{Actor: actor, Origin: origin, Ops: ops})
		return err
	})
	return meta, rev, err
}

func metaOf(cvID uuid.UUID, s State) cv.Meta {
	return cv.Meta{ID: cvID, Title: s.Title, TemplateID: s.TemplateID}
}

// History is the feed, newest first.
func (e *Editor) History(ctx context.Context, cvID uuid.UUID, userID int64) ([]Revision, error) {
	return e.repo.List(ctx, cvID, userID, feedLimit)
}
