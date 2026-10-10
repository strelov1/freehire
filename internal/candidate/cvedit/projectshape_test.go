package cvedit

import (
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/cv"
)

func TestCommitRefusesAgentJobDatedProjectName(t *testing.T) {
	for _, name := range []string{
		"Senior Engineer, Acme Corp (2020 - 2023)",
		"Staff Engineer at Acme — 2019–Present",
		"Contractor, Acme 2018 to current",
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.state.Projects = []cv.Project{{Name: "freehire"}}
			e, _ := newEditor(repo, &bank{})

			err := agentEdit(t, e, Op{
				Kind: OpSet, Path: mustParse(t, "projects[0].name"),
				Value: name, EvidenceID: "banked",
			})
			if !errors.Is(err, ErrProjectLooksLikeJob) {
				t.Fatalf("Commit(%q) = %v, want ErrProjectLooksLikeJob", name, err)
			}
			if !strings.Contains(err.Error(), "experience") {
				t.Fatalf("refusal %q does not name experience[] as the fix", err)
			}
			if repo.saves != 0 || len(repo.revisions) != 0 {
				t.Fatal("a refused edit must not save or file a revision")
			}
			if got := repo.state.Projects[0].Name; got != "freehire" {
				t.Fatalf("project name = %q, want the original kept", got)
			}
		})
	}
}

func TestCommitAllowsAgentProjectNameWithoutDateRange(t *testing.T) {
	for _, name := range []string{
		"Freelance Consulting, Acme Corp",
		"Redesign for Acme Corp",
		"Game Jam 2021",
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.state.Projects = []cv.Project{{Name: "freehire"}}
			e, _ := newEditor(repo, &bank{})

			err := agentEdit(t, e, Op{
				Kind: OpSet, Path: mustParse(t, "projects[0].name"),
				Value: name, EvidenceID: "banked",
			})
			if err != nil {
				t.Fatalf("Commit(%q) = %v, want no error", name, err)
			}
			if got := repo.state.Projects[0].Name; got != name {
				t.Fatalf("project name = %q, want %q", got, name)
			}
		})
	}
}

func TestCommitAllowsAgentProjectBulletMentioningAYear(t *testing.T) {
	repo := newFakeRepo()
	repo.state.Projects = []cv.Project{{Name: "freehire", Bullets: []string{"Shipped v1"}}}
	e, _ := newEditor(repo, &bank{})

	err := agentEdit(t, e, Op{
		Kind: OpSet, Path: mustParse(t, "projects[0].bullets[0]"),
		Value: "Cut P95 latency from 20s to 1s in 2021 - 2022", EvidenceID: "banked",
	})
	if err != nil {
		t.Fatalf("Commit = %v, want no error — only a project's name is checked, never its bullets", err)
	}
}

func TestCommitAllowsCandidateJobDatedProjectName(t *testing.T) {
	repo := newFakeRepo()
	repo.state.Projects = []cv.Project{{Name: "freehire"}}
	e := NewEditor(repo, nil)

	commitSet(t, e, repo, ActorCandidate, OriginEditor,
		"projects[0].name", "Senior Engineer, Acme Corp (2020 - 2023)")
	if got := repo.state.Projects[0].Name; got != "Senior Engineer, Acme Corp (2020 - 2023)" {
		t.Fatalf("project name = %q, want the candidate's own text kept — this check is agent-only", got)
	}
}

func TestCommitRefusesWholeBatchWhenOneProjectIsJobDated(t *testing.T) {
	repo := newFakeRepo()
	repo.state.Projects = []cv.Project{{Name: "freehire"}, {Name: "sandrock"}}
	e, _ := newEditor(repo, &bank{})

	err := agentEdit(t, e,
		Op{Kind: OpSet, Path: mustParse(t, "projects[0].name"),
			Value: "Senior Engineer, Acme Corp (2020 - 2023)", EvidenceID: "banked"},
		Op{Kind: OpSet, Path: mustParse(t, "projects[1].name"),
			Value: "Sandrock Trading Bot", EvidenceID: "banked"},
	)
	if !errors.Is(err, ErrProjectLooksLikeJob) {
		t.Fatalf("Commit = %v, want ErrProjectLooksLikeJob", err)
	}
	if repo.state.Projects[0].Name != "freehire" || repo.state.Projects[1].Name != "sandrock" {
		t.Fatal("a refused batch must leave BOTH projects untouched, not just the offending one")
	}
}
