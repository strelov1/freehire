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
}

// Tx is the transactional view of one CV: everything a commit does, it does through here.
type Tx interface {
	State(ctx context.Context) (State, time.Time, error)
	Save(ctx context.Context, state State) (cv.Meta, error)
	Newest(ctx context.Context) (Revision, bool, error)
	Revision(ctx context.Context, id uuid.UUID) (Revision, bool, error)
	Insert(ctx context.Context, rev Revision) (Revision, error)
	Amend(ctx context.Context, id uuid.UUID, ops []Op, title, note string) (Revision, error)
	MarkReverted(ctx context.Context, id uuid.UUID) (bool, error)
	InBatch(ctx context.Context, batchID uuid.UUID) ([]Revision, error)
	// Feed is the CV's revision log, newest first, up to limit — what undo reads to check
	// whether a foreign edit has reshaped a list since the revision(s) it is about to reverse.
	Feed(ctx context.Context, limit int32) ([]Revision, error)
	Trim(ctx context.Context, keep int32) error
}

// ErrNothingToUndo is returned when an undo names a revision that has already been undone,
// or a batch that never edited this CV.
var ErrNothingToUndo = errors.New("cvedit: nothing to undo")

// ErrCommitDocumentAgentUnsupported is returned when CommitDocument is asked to record a
// whole-document save on the agent's behalf. See CommitDocument's own comment for why.
var ErrCommitDocumentAgentUnsupported = errors.New("cvedit: CommitDocument does not support the agent actor")

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
	// refuseListCap refuses a commit that would leave more than cv.MaxBullets
	// non-empty bullets on one role — otherwise Sanitize silently drops the rest.
	// On by default; SetRefuseListCap(false) is the ops escape hatch.
	refuseListCap bool
}

// NewEditor builds the editor over its repository and the gate an agent's claims answer
// to. The gate is supplied here and nowhere else, so an editor that admits an uncited
// agent claim cannot be produced by assembling things in the wrong order.
//
// A nil gate means CANDIDATE-authored editing only: requireEvidence exempts the candidate,
// who is the source the bank exists to record. It is what the candidate-only test editors
// pass, and it is not a configuration an agent write path may be built on.
func NewEditor(repo Repository, gate EvidenceGate) *Editor {
	return &Editor{repo: repo, policy: DefaultPolicy(), gate: gate, now: time.Now, refuseListCap: true}
}

// SetRefuseListCap turns the over-cap refuse on or off. When off, Sanitize may drop
// trailing bullets again — the pre-refuse behaviour. Returns the editor so callers can
// chain it at construction (e.g. NewEditor(...).SetRefuseListCap(cfg.On)).
func (e *Editor) SetRefuseListCap(on bool) *Editor {
	e.refuseListCap = on
	return e
}

// Commit applies a batch and records it. The document and its revision are written in one
// transaction against a locked row, so a reader never sees one without the other.
func (e *Editor) Commit(ctx context.Context, cvID uuid.UUID, userID int64, ch Change) (cv.Meta, Revision, error) {
	var (
		meta cv.Meta
		rev  Revision
	)
	// Both checks run BEFORE the transaction opens. The policy needs nothing but the batch,
	// and the gate reads the experience bank on its own connection — asking it while holding
	// the CV row lock borrows a second connection from the pool for the length of a commit,
	// which is the shape a pool-exhaustion deadlock takes under load.
	if err := e.authorize(ctx, userID, ch); err != nil {
		return cv.Meta{}, Revision{}, err
	}
	err := e.repo.Edit(ctx, cvID, userID, func(ctx context.Context, tx Tx) error {
		var err error
		meta, rev, err = e.commit(ctx, tx, cvID, userID, ch)
		return err
	})
	return meta, rev, err
}

// authorize answers "may this actor make this change at all" — a question about the batch, not
// about the document, so it is settled before anything is locked.
func (e *Editor) authorize(ctx context.Context, userID int64, ch Change) error {
	if len(ch.Ops) == 0 {
		return fmt.Errorf("%w: a change with no operations", ErrInvalidOp)
	}
	if err := e.policy.Allows(ch.Actor, ch.Ops); err != nil {
		return err
	}
	return e.requireEvidence(ctx, userID, ch)
}

// commit assumes authorize has already passed: it is the part that needs the locked row.
func (e *Editor) commit(ctx context.Context, tx Tx, cvID uuid.UUID, userID int64, ch Change) (cv.Meta, Revision, error) {
	before, baseVersion, err := tx.State(ctx)
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}
	applied, applyInverse, err := Apply(before, ch.Ops)
	if err != nil {
		return cv.Meta{}, Revision{}, err
	}

	// Sanitize after applying and before storing, exactly as the whole-document path always
	// has: a change states what was asked for, and the sanitizer decides what is kept.
	after := applied
	after.Sanitize()

	// Sanitize used to silently keep the first MaxBullets and drop the rest. An
	// agent insert into a full list then looked like a successful "add" while
	// original trailing bullets vanished. Refuse that class of loss before Save
	// unless an operator turned the guard off (SetRefuseListCap(false)).
	if e.refuseListCap {
		if err := refuseIfSanitizeDropsContent(applied, after); err != nil {
			return cv.Meta{}, Revision{}, err
		}
	}

	// Which inverse to store depends on whether the sanitizer moved anything.
	//
	// Apply's inverse is the precise one: it reverses each operation in kind, so undoing a
	// reorder is one `move` back rather than a rewrite of every entry it touched. But it was
	// computed against the pre-sanitized result, and the sanitizer drops empty entries and
	// whitespace-only bullets — every index after such a drop shifts, and the inverse then
	// removes the wrong element.
	//
	// So: keep Apply's inverse when the sanitizer changed nothing (the overwhelming majority,
	// and the only case where a `move` survives at all — the differ has no move in its
	// vocabulary). Fall back to the diff when it did, where precision is already lost but
	// correctness is not.
	inverse := applyInverse
	if !Equal(applied, after) {
		inverse = Diff(after, before)
	}

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
		//
		// The note IS replaced along with the title: it is the stated reason for the newest
		// batch's content, and a coalesced save already carries that content whole rather than
		// accumulating it — restating anything less would attribute the current text to a
		// reason that no longer describes it.
		return tx.Amend(ctx, newest.ID, ch.Ops, title, ch.Note)
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
	// A second agent turn is a second run. Folding its first edit into the previous turn's
	// revision would file it under that run's batch, and "undo the run" would miss it.
	if newest.BatchID != ch.BatchID {
		return false
	}
	// Only `set` may be folded. Replacing the operations is right for it — each save carries
	// the place's current value — and wrong for the rest: two inserts at the same position are
	// two additions, and keeping only the second leaves the first recorded nowhere and
	// undoable by nothing.
	if !allSets(newest.Ops) || !allSets(ch.Ops) {
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

func allSets(ops []Op) bool {
	for _, op := range ops {
		if op.Kind != OpSet {
			return false
		}
	}
	return true
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
//
// Unlike Commit, authorize() runs INSIDE the locked transaction here, because the operations
// it checks do not exist until Diff has compared the incoming document against the state the
// lock protects — there is nothing to authorize before that. Commit's ordering exists to keep
// the evidence gate's bank read (a second pooled connection) from running while the CV row is
// locked, which is the shape of a pool-exhaustion deadlock under load; requireEvidence only
// ever makes that read for ActorAgent. So this refuses the agent actor outright rather than
// trusting every future caller to keep passing ActorCandidate/ActorSystem, which is what made
// today's ordering safe by convention rather than by construction.
func (e *Editor) CommitDocument(ctx context.Context, cvID uuid.UUID, userID int64,
	actor Actor, origin Origin, next State) (cv.Meta, Revision, error) {

	if actor == ActorAgent {
		return cv.Meta{}, Revision{}, ErrCommitDocumentAgentUnsupported
	}

	var (
		meta cv.Meta
		rev  Revision
	)
	err := e.repo.Edit(ctx, cvID, userID, func(ctx context.Context, tx Tx) error {
		before, _, err := tx.State(ctx)
		if err != nil {
			return err
		}
		// Refuse BEFORE Sanitize can truncate what the caller sent. Sanitize below keeps the
		// first cv.MaxBullets and drops the rest; checking after it ran would see a document
		// that already lost the excess and could never trip ErrListCap. This is the same
		// guard commit() applies to an agent's Ops batch, extended to a whole-document save —
		// PUT /me/cvs/:id (the editor's autosave) and Reset from résumé both go through
		// here, and a pasted section or a bank seed can carry more than the cap.
		if e.refuseListCap {
			if err := refuseIfSanitizeDropsContent(next, State{}); err != nil {
				return err
			}
		}
		// Sanitize the incoming state first, so the diff is against what would be stored
		// rather than against what was sent — otherwise a value the sanitizer clamps would
		// show up as an edit on every save.
		next.Sanitize()

		ops := Diff(before, next)
		if len(ops) == 0 {
			meta = metaOf(cvID, before)
			return nil
		}
		change := Change{Actor: actor, Origin: origin, Ops: ops}
		// The operations are derived here, so this is the first moment they can be checked.
		// The actor guard above guarantees ActorAgent never reaches this call, and the gate
		// does not apply to candidate or system, so no bank read ever happens under the lock.
		if err := e.authorize(ctx, userID, change); err != nil {
			return err
		}
		meta, rev, err = e.commit(ctx, tx, cvID, userID, change)
		return err
	})
	return meta, rev, err
}

func metaOf(cvID uuid.UUID, s State) cv.Meta {
	return cv.Meta{ID: cvID, Title: s.Title, TemplateID: s.TemplateID}
}

// Seed opens a CV's history with a system revision: the copy exists, and this is where its
// feed starts. It carries no operations and no inverse, because there is nothing to undo —
// undoing "this CV was created" is deleting it, which is a different action with its own
// button.
//
// Without it a tailored copy's history would begin mid-story, with the first edit and no
// account of where the document came from.
func (e *Editor) Seed(ctx context.Context, cvID uuid.UUID, userID int64, title string) (Revision, error) {
	var rev Revision
	err := e.repo.Edit(ctx, cvID, userID, func(ctx context.Context, tx Tx) error {
		_, baseVersion, err := tx.State(ctx)
		if err != nil {
			return err
		}
		rev, err = tx.Insert(ctx, Revision{
			CVID:        cvID,
			Actor:       ActorSystem,
			Origin:      OriginImport,
			Title:       title,
			BaseVersion: baseVersion,
		})
		return err
	})
	return rev, err
}

// History is the feed, newest first.
func (e *Editor) History(ctx context.Context, cvID uuid.UUID, userID int64) ([]Revision, error) {
	return e.repo.List(ctx, cvID, userID, feedLimit)
}
