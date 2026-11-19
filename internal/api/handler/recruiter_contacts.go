package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"slices"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/contactfinder"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// recruiterContactHandlers serves the people who recruit for a job's company. The contacts are
// cached per job, so a second viewer costs no lookup; every serve from the cache first asks the
// finder which addresses were asked to be removed since.
type recruiterContactHandlers struct {
	queries *db.Queries
	plans   *plan.Store
	// finder is nil when CONTACT_FINDER_URL is unset: the feature is off and the route a 404.
	finder *contactfinder.Client
}

const (
	recruiterContactsTTL      = 30 * 24 * time.Hour
	recruiterContactsEmptyTTL = 7 * 24 * time.Hour
)

func (h *recruiterContactHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Post("/jobs/:slug/recruiter-contacts", mw.key, mw.paidAI(plan.FeatureRecruiterContact), contract)
}

// FindRecruiterContacts charges the first view of a job (ref "job:<id>", so looking again is
// free) and gives the charge back when the answer is empty or the lookup fails.
func (h *recruiterContactHandlers) FindRecruiterContacts(ctx context.Context, params apigen.FindRecruiterContactsParams) (apigen.FindRecruiterContactsRes, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if h.finder == nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "recruiter contacts are not available")
	}
	job, err := h.queries.GetJobRecruiterDomain(ctx, params.Slug)
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404
	}
	if job.Domain == "" {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "this company has no website on record")
	}

	ref := "job:" + strconv.FormatInt(job.ID, 10)
	d, err := h.plans.Consume(ctx, userID, plan.FeatureRecruiterContact, ref)
	if isRefusal(err) {
		return decisionRefusal(d).cv()
	}
	if err != nil {
		// Fails closed, unlike the AI meters: an unmeasured view is a paid lookup at a third party,
		// and the daily count is what the two plans differ on.
		log.Printf("plan: charging recruiter contacts for user %d: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "recruiter contacts are unavailable right now")
	}

	// shortcut: the same user sending twice at once shares one charge; if one request fails
	// after the other served contacts, the release makes that view free and a reopen pays
	// again. A per-user/job lock if double sends turn out common.
	contacts, err := h.contacts(ctx, job.ID, job.Domain, userID)
	if (err != nil || len(contacts) == 0) && d.Charge > 0 {
		h.release(userID, ref)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		log.Printf("recruiter-contacts: job %d: %v", job.ID, err)
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "recruiter contacts are unavailable right now, try again later")
	}

	out := make([]apigen.RecruiterContact, 0, len(contacts))
	for _, c := range contacts {
		out = append(out, apigen.RecruiterContact{
			Name: c.Name, Title: c.Title, Email: c.Email,
			Confidence: apigen.RecruiterContactConfidence(c.Confidence),
		})
	}
	return &apigen.FindRecruiterContactsOK{Data: out}, nil
}

// contacts serves the job's cached contacts while fresh (an empty answer stays fresh for less,
// since a company's recruiters may appear), otherwise looks them up and caches the answer.
func (h *recruiterContactHandlers) contacts(ctx context.Context, jobID int64, domain string, userID int64) ([]contactfinder.Contact, error) {
	row, err := h.queries.GetJobRecruiterContacts(ctx, jobID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, err
	default:
		var cached []contactfinder.Contact
		if err := json.Unmarshal(row.Contacts, &cached); err != nil {
			return nil, err
		}
		age := time.Since(row.FoundAt.Time)
		if age < recruiterContactsTTL && (len(cached) > 0 || age < recruiterContactsEmptyTTL) {
			return h.withoutSuppressed(ctx, jobID, cached, row.FoundAt.Time)
		}
	}

	// shortcut: two first viewers at once both look up; a per-job lock if lookups get costly.
	found, err := h.finder.Find(ctx, domain, strconv.FormatInt(userID, 10))
	if err != nil {
		return nil, err
	}
	// The lookup is spent and recorded by the finder already: a failed cache write is not the
	// viewer's problem.
	h.store(ctx, jobID, found, time.Now())
	return found, nil
}

// withoutSuppressed drops cached addresses removed on request since they were found, and
// rewrites the cache without them. A check that fails is a failure: a removal must hold.
func (h *recruiterContactHandlers) withoutSuppressed(ctx context.Context, jobID int64, cached []contactfinder.Contact, foundAt time.Time) ([]contactfinder.Contact, error) {
	if len(cached) == 0 {
		return cached, nil
	}
	emails := make([]string, len(cached))
	for i, c := range cached {
		emails[i] = c.Email
	}
	suppressed, err := h.finder.Suppressed(ctx, emails)
	if err != nil {
		return nil, err
	}
	kept := slices.DeleteFunc(cached, func(c contactfinder.Contact) bool { return suppressed[c.Email] })
	if len(kept) != len(emails) {
		h.store(ctx, jobID, kept, foundAt)
	}
	return kept, nil
}

func (h *recruiterContactHandlers) store(ctx context.Context, jobID int64, contacts []contactfinder.Contact, foundAt time.Time) {
	raw, err := json.Marshal(contacts)
	if err == nil {
		err = h.queries.PutJobRecruiterContacts(ctx, db.PutJobRecruiterContactsParams{
			JobID: jobID, Contacts: raw, FoundAt: pgconv.Instant(foundAt),
		})
	}
	if err != nil {
		log.Printf("recruiter-contacts: caching job %d: %v", jobID, err)
	}
}

// release gives back a charge that bought nothing, on a detached context so a cancelled
// request still releases.
func (h *recruiterContactHandlers) release(userID int64, ref string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.plans.Release(ctx, userID, plan.FeatureRecruiterContact, ref); err != nil {
		log.Printf("plan: releasing recruiter contacts for user %d: %v", userID, err)
	}
}
