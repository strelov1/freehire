package cvedit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// bank is a stand-in for the experience bank: it knows one publishable claim and one the
// model inferred for itself.
type bank struct{ calls int }

var errNotPublishable = errors.New("that achievement is recorded as your own reading rather than the candidate's statement")

func (b *bank) Publishable(_ context.Context, _ int64, evidenceID string) error {
	b.calls++
	switch evidenceID {
	case "banked":
		return nil
	case "inferred":
		return errNotPublishable
	default:
		return errors.New("no banked achievement with that id")
	}
}

func agentEdit(t *testing.T, e *Editor, ops ...Op) error {
	t.Helper()
	_, _, err := e.Commit(context.Background(), uuid.Nil, 1, Change{
		Actor: ActorAgent, Origin: OriginTailorAgent, Ops: ops,
	})
	return err
}

func TestTheAgentIsRefusedTheCandidatesOwnFields(t *testing.T) {
	for _, path := range []string{"header.full_name", "header.email", "header.phone", "header.links[0]", "title", "template_id"} {
		t.Run(path, func(t *testing.T) {
			repo := newFakeRepo()
			repo.state.Header.Links = []string{"https://example.com"}
			e, _ := newEditor(repo, nil)

			err := agentEdit(t, e, Op{Kind: OpSet, Path: mustParse(t, path), Value: "taken over"})
			if !errors.Is(err, ErrForbiddenPath) {
				t.Fatalf("agent editing %s returned %v, want ErrForbiddenPath", path, err)
			}
			// The refusal has to say what the agent may do instead: for a model the error
			// message is its only route to correcting itself inside the turn.
			if !strings.Contains(err.Error(), "experience") {
				t.Fatalf("refusal %q does not name what the agent may edit", err)
			}
			if repo.saves != 0 || len(repo.revisions) != 0 {
				t.Fatal("a refused edit wrote something")
			}
		})
	}
}

func TestTheCandidateEditsTheSameFieldsFreely(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	commitSet(t, e, repo, ActorCandidate, OriginEditor, "header.email", "ada@lovelace.dev")

	if repo.state.Header.Email != "ada@lovelace.dev" {
		t.Fatalf("email = %q, want the candidate's edit", repo.state.Header.Email)
	}
}

func TestAClaimAboutTheCandidateNeedsEvidence(t *testing.T) {
	// Every one of these puts a new assertion on the page. The last two are the hole the
	// named-op vocabulary had: a technology or a skill asserts exactly what a bullet does.
	for _, path := range []string{
		"summary",
		"experience[0].summary",
		"experience[0].bullets[0]",
		"experience[0].stack[0]",
		"skills[0].items[0]",
	} {
		t.Run(path, func(t *testing.T) {
			repo := newFakeRepo()
			e, _ := newEditor(repo, &bank{})

			err := agentEdit(t, e, Op{Kind: OpSet, Path: mustParse(t, path), Value: "Ran Kubernetes in production"})
			if !errors.Is(err, ErrEvidenceRequired) {
				t.Fatalf("uncited write to %s returned %v, want ErrEvidenceRequired", path, err)
			}
			if !strings.Contains(err.Error(), "experience_search") {
				t.Fatalf("refusal %q does not say how to obtain a citation", err)
			}
		})
	}
}

func TestTheCandidateWritesTheirOwnBulletWithoutACitation(t *testing.T) {
	repo := newFakeRepo()
	b := &bank{}
	e, _ := newEditor(repo, b)

	// The rule is that a MODEL's inference must not reach the page. The candidate writing
	// about their own career is the source the bank exists to record — requiring a citation
	// here would make their own editor unusable.
	commitSet(t, e, repo, ActorCandidate, OriginEditor, "experience[0].bullets[0]", "Ran Kubernetes in production")

	if repo.state.Experience[0].Bullets[0] != "Ran Kubernetes in production" {
		t.Fatalf("bullet = %q, want the candidate's own words", repo.state.Experience[0].Bullets[0])
	}
	if b.calls != 0 {
		t.Fatalf("the bank was asked %d times about the candidate's own claim", b.calls)
	}
}

func TestACitedClaimIsWritten(t *testing.T) {
	repo := newFakeRepo()
	b := &bank{}
	e, _ := newEditor(repo, b)

	if err := agentEdit(t, e, Op{
		Kind: OpSet, Path: mustParse(t, "experience[0].bullets[0]"),
		Value: "Ran Kubernetes in production", EvidenceID: "banked",
	}); err != nil {
		t.Fatalf("cited write refused: %v", err)
	}
	if repo.state.Experience[0].Bullets[0] != "Ran Kubernetes in production" {
		t.Fatalf("bullet not written: %q", repo.state.Experience[0].Bullets[0])
	}
	if b.calls != 1 {
		t.Fatalf("the bank was asked %d times, want once", b.calls)
	}
}

func TestEvidenceTheModelInferredCannotBeCited(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, &bank{})

	err := agentEdit(t, e, Op{
		Kind: OpSet, Path: mustParse(t, "experience[0].bullets[0]"),
		Value: "Ran Kubernetes in production", EvidenceID: "inferred",
	})
	if !errors.Is(err, errNotPublishable) {
		t.Fatalf("returned %v, want the bank's own refusal", err)
	}
	if repo.saves != 0 {
		t.Fatal("a refused claim was written")
	}
}

func TestOneUncitedOperationRefusesTheWholeBatch(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, &bank{})

	err := agentEdit(t, e,
		Op{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[0]"), Value: "Ran the cluster", EvidenceID: "banked"},
		Op{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[1]"), Value: "Mentored two juniors", EvidenceID: "banked"},
		Op{Kind: OpSet, Path: mustParse(t, "experience[1].bullets[0]"), Value: "Invented the wheel"},
	)
	if !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("returned %v, want ErrEvidenceRequired", err)
	}
	if repo.saves != 0 || len(repo.revisions) != 0 {
		t.Fatal("a batch with one uncited claim wrote something")
	}
	if repo.state.Experience[0].Bullets[0] != "Shipped it" {
		t.Fatalf("the cited operations were applied anyway: %q", repo.state.Experience[0].Bullets[0])
	}
}

func TestRearrangingNeedsNoEvidence(t *testing.T) {
	repo := newFakeRepo()
	b := &bank{}
	e, _ := newEditor(repo, b)
	to := 0

	if err := agentEdit(t, e,
		Op{Kind: OpMove, Path: mustParse(t, "experience[0].bullets[1]"), To: &to},
		Op{Kind: OpRemove, Path: mustParse(t, "experience[1].bullets[0]")},
	); err != nil {
		t.Fatalf("rearranging refused: %v", err)
	}
	if b.calls != 0 {
		t.Fatalf("the bank was asked %d times for operations that assert nothing", b.calls)
	}
}

func TestUndoingRestoresTextWithoutACitation(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, &bank{})
	cvID := repo.cvID

	_, rev, err := e.Commit(context.Background(), cvID, 1, Change{
		Actor: ActorAgent, Origin: OriginTailorAgent,
		Ops: []Op{{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[0]"),
			Value: "Ran Kubernetes in production", EvidenceID: "banked"}},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// The inverse writes into a gated path too — it puts the candidate's own earlier words
	// back. Requiring a citation for that would make the gate refuse to undo itself.
	if _, _, err := e.Revert(context.Background(), cvID, 1, rev.ID); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if repo.state.Experience[0].Bullets[0] != "Shipped it" {
		t.Fatalf("bullet = %q, want the original text back", repo.state.Experience[0].Bullets[0])
	}
}

// Denying a leaf is not enough: writing the CONTAINER writes everything under it. The path
// vocabulary published to the model offers `header` alongside `header.email`, and coercing a
// whole object into it replaces every contact identifier at once.
func TestTheAgentIsRefusedTheContainerOfADeniedField(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	err := agentEdit(t, e, Op{Kind: OpSet, Path: mustParse(t, "header"), Value: map[string]any{
		"full_name": "Attacker", "email": "attacker@example.com", "phone": "+1 000",
	}})
	if !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("agent writing the whole header returned %v, want ErrForbiddenPath", err)
	}
	if repo.state.Header.Email != "ada@example.com" {
		t.Fatalf("the contact block was rewritten: %+v", repo.state.Header)
	}
}

// The same shape one level up: an operation that writes a container writes the claims inside
// it, so it has to answer for them. This is the hole the gate exists to close, reopened by
// addressing `experience[0]` instead of `experience[0].bullets[0]`.
func TestWritingAContainerOfClaimsNeedsEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		op    Op
		value any
	}{
		{"a whole experience entry", Op{Kind: OpSet, Path: "experience[0]"},
			map[string]any{"role": "Chief Kubernetes Officer", "bullets": []string{"Invented Kubernetes"}}},
		{"a whole bullet list", Op{Kind: OpSet, Path: "experience[0].bullets"}, []string{"Invented Kubernetes"}},
		{"a whole stack line", Op{Kind: OpSet, Path: "experience[0].stack"}, []string{"Kubernetes"}},
		{"a whole skill group", Op{Kind: OpSet, Path: "skills[0]"},
			map[string]any{"group": "Cloud", "items": []string{"Kubernetes"}}},
		{"a whole skill list", Op{Kind: OpSet, Path: "skills[0].items"}, []string{"Kubernetes"}},
		{"the whole experience section", Op{Kind: OpSet, Path: "experience"},
			[]map[string]any{{"role": "CTO", "bullets": []string{"Invented Kubernetes"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			e, _ := newEditor(repo, &bank{})

			op := tc.op
			op.Value = tc.value
			err := agentEdit(t, e, op)
			if !errors.Is(err, ErrEvidenceRequired) {
				t.Fatalf("uncited write to %s returned %v, want ErrEvidenceRequired", op.Path, err)
			}
			if repo.saves != 0 {
				t.Fatal("an uncited claim was written")
			}
		})
	}
}

// A degree nobody earned is a bigger lie than a bullet nobody wrote, and it was landing
// uncited: the gate listed the places that carry a claim, and education, certifications and
// languages were simply not on the list. Nor were an entry's own identity fields — the role,
// the employer, the dates.
func TestFabricatedCredentialsNeedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		op    Op
		value any
	}{
		{"a degree", Op{Kind: OpInsert, Path: "education[0]"},
			map[string]any{"institution": "Stanford", "degree": "PhD"}},
		{"the whole education section", Op{Kind: OpSet, Path: "education"},
			[]map[string]any{{"institution": "MIT", "degree": "MSc"}}},
		{"a certification", Op{Kind: OpInsert, Path: "certifications[0]"},
			map[string]any{"name": "AWS Solutions Architect Professional"}},
		{"a language", Op{Kind: OpInsert, Path: "languages[0]"},
			map[string]any{"name": "Japanese", "level": "native"}},
		{"an inflated job title", Op{Kind: OpSet, Path: "experience[0].role"}, "VP of Engineering"},
		{"an employer never worked for", Op{Kind: OpSet, Path: "experience[0].company"}, "Google"},
		{"a papered-over gap", Op{Kind: OpSet, Path: "experience[0].start"}, "2015"},
		{"a skill group's name", Op{Kind: OpSet, Path: "skills[0].group"}, "Distributed Systems"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			e, _ := newEditor(repo, &bank{})

			op := tc.op
			op.Value = tc.value
			if err := agentEdit(t, e, op); !errors.Is(err, ErrEvidenceRequired) {
				t.Fatalf("uncited write to %s returned %v, want ErrEvidenceRequired", op.Path, err)
			}
			if repo.saves != 0 {
				t.Fatal("an uncited claim was written")
			}
		})
	}
}

// Presentation is not a claim: the candidate's own layout choices assert nothing about their
// career, and gating them would leave the agent unable to fit a CV onto one page.
func TestPresentationNeedsNoEvidence(t *testing.T) {
	repo := newFakeRepo()
	b := &bank{}
	e, _ := newEditor(repo, b)

	for _, op := range []Op{
		{Kind: OpSet, Path: "style.font_size", Value: 11.0},
		{Kind: OpSet, Path: "margins.left", Value: 0.75},
	} {
		if err := agentEdit(t, e, op); err != nil {
			t.Fatalf("%s was gated: %v", op.Path, err)
		}
	}
	if b.calls != 0 {
		t.Fatalf("the bank was asked %d times about presentation", b.calls)
	}
}
