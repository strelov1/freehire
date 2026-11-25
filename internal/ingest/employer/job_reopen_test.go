package employer

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
)

func TestReopenVacancy_UndoesTheEmployersOwnClose(t *testing.T) {
	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
	jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
	if err := s.CloseVacancy(context.Background(), 1, "go-engineer"); err != nil {
		t.Fatalf("CloseVacancy: %v", err)
	}
	if err := s.ReopenVacancy(context.Background(), 1, "go-engineer"); err != nil {
		t.Fatalf("ReopenVacancy: %v", err)
	}
	if jobs.closed["go-engineer"] {
		t.Error("the vacancy is still closed")
	}
}

func TestDeleteVacancy(t *testing.T) {
	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
	jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
	if err := s.DeleteVacancy(context.Background(), 1, "go-engineer"); err != nil {
		t.Fatalf("DeleteVacancy: %v", err)
	}
	if err := s.DeleteVacancy(context.Background(), 1, "go-engineer"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("second delete: err = %v, want ErrJobNotFound", err)
	}
}

func TestReopenVacancy_Refusals(t *testing.T) {
	t.Run("another employer's vacancy", func(t *testing.T) {
		jobs := newFakeJobRepo()
		s := activeService(t, 2, jobs, &fakeMinter{repo: jobs})
		jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
		jobs.closed["go-engineer"] = true
		if err := s.ReopenVacancy(context.Background(), 2, "go-engineer"); !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("err = %v, want ErrJobNotFound", err)
		}
	})
	t.Run("an open vacancy, or one closed by someone else", func(t *testing.T) {
		jobs := newFakeJobRepo()
		s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
		jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
		if err := s.ReopenVacancy(context.Background(), 1, "go-engineer"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})
}

// Search keeps out a posting with no description, so an employer publishing one would see
// it vanish without a word. The form refuses it instead — on create in either mode, and on
// an edit that would blank it.
func TestVacancy_ADescriptionIsRequired(t *testing.T) {
	for name, in := range map[string]VacancyInput{
		"external, none":     {URL: "https://acme.test/jobs/1", Title: "Go Engineer"},
		"on-site, blank":     {ApplyMode: ApplyFreehire, Title: "Go Engineer", Description: "  "},
		"on-site, tags only": {ApplyMode: ApplyFreehire, Title: "Go Engineer", Description: "<p> </p>"},
	} {
		t.Run(name, func(t *testing.T) {
			jobs := newFakeJobRepo()
			minter := &fakeMinter{repo: jobs}
			s := activeService(t, 1, jobs, minter)
			if _, _, err := s.CreateVacancy(context.Background(), 1, in); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
			if len(minter.calls) != 0 {
				t.Error("a vacancy with no description must not be minted")
			}
		})
	}

	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
	jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
	blank := " "
	if _, _, err := s.UpdateVacancy(context.Background(), 1, "go-engineer", VacancyPatch{Description: &blank}); !errors.Is(err, ErrInvalid) {
		t.Errorf("blanking the description: err = %v, want ErrInvalid", err)
	}
}
