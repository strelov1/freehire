package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
)

// usageHistoryLimit bounds the history to a recent window. A day's entries are few; a cursor
// API can go behind the same endpoint if it ever grows.
const usageHistoryLimit = 100

// usageHistoryEntry is one ledger row ready for display: which feature, which day, and what
// it was spent on when that can be named.
type usageHistoryEntry struct {
	Feature   string    `json:"feature"`
	Day       string    `json:"day"`
	Kind      string    `json:"kind"`
	Label     string    `json:"label"`
	Subtitle  string    `json:"subtitle,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// usageEntryLabel is the display label for a ledger row. subject is the resolved vacancy for
// an entry that names one, or "" when it names nothing resolvable.
func usageEntryLabel(kind string, feature plan.Feature, subject string) (label, subtitle string) {
	if kind == "release" {
		return "Returned — nothing was produced", subject
	}
	switch feature {
	case plan.FeatureFit:
		return "Job analysis", subject
	case plan.FeatureTailor:
		return "CV editing session", subject
	case plan.FeatureAssistant:
		return "Assistant message", ""
	case plan.FeatureDictation:
		return "Dictation", ""
	default:
		return "Used", ""
	}
}

// GetMyPlanHistory returns the caller's usage entries newest first, each labelled for
// display. Cookie or API key; never calls the LLM. References are resolved in two batch
// lookups — a job analysis names a job id, a tailoring session names a session whose CV
// carries the vacancy — so the list reads in plain terms.
func (h *planHandlers) GetMyPlanHistory(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := h.queries.ListUsageLedger(c.Context(), db.ListUsageLedgerParams{
		UserID: userID, Limit: usageHistoryLimit,
	})
	if err != nil {
		return err
	}
	subjects, err := h.resolveUsageSubjects(c, userID, rows)
	if err != nil {
		return err
	}

	entries := make([]usageHistoryEntry, 0, len(rows))
	for _, r := range rows {
		feature := plan.Feature(r.Feature)
		label, subtitle := usageEntryLabel(r.Kind, feature, subjects[subjectKey(r.Feature, r.Ref.String)])
		entries = append(entries, usageHistoryEntry{
			Feature:   r.Feature,
			Day:       r.Day.Time.Format(time.DateOnly),
			Kind:      r.Kind,
			Label:     label,
			Subtitle:  subtitle,
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return c.JSON(fiber.Map{"data": entries})
}

// resolveUsageSubjects batch-resolves the vacancies the entries name, keyed by
// subjectKey(feature, ref).
//
// The two features name different things and neither can be parsed as the other: a job
// analysis carries a job's numeric id, a tailoring charge carries '<session>#n'. The
// feature is part of the key so a job id and a session that happen to look alike cannot
// collide. A reference whose subject was deleted is simply absent, and the caller falls
// back to a generic label.
func (h *planHandlers) resolveUsageSubjects(c *fiber.Ctx, userID int64, rows []db.ListUsageLedgerRow) (map[string]string, error) {
	var jobRefs []int64
	var sessionRefs []string
	// sessionByRef remembers which ledger reference each session id came from, because the
	// reference carries the '#n' suffix the session id does not.
	sessionByRef := map[string]string{}

	for _, r := range rows {
		if !r.Ref.Valid {
			continue
		}
		switch plan.Feature(r.Feature) {
		case plan.FeatureFit:
			id, err := strconv.ParseInt(r.Ref.String, 10, 64)
			if err != nil {
				continue // a reference that is not a job id never resolves
			}
			jobRefs = append(jobRefs, id)
		case plan.FeatureTailor:
			session, _, found := strings.Cut(r.Ref.String, "#")
			if !found || session == "" {
				continue
			}
			sessionRefs = append(sessionRefs, session)
			sessionByRef[r.Ref.String] = session
		}
	}

	subjects := make(map[string]string)
	if len(jobRefs) > 0 {
		jobs, err := h.queries.ListJobLabelsByIDs(c.Context(), jobRefs)
		if err != nil {
			return nil, err
		}
		for _, j := range jobs {
			subjects[subjectKey(string(plan.FeatureFit), strconv.FormatInt(j.ID, 10))] = jobLabel(j.Title, j.PublicSlug)
		}
	}
	if len(sessionRefs) > 0 {
		cvs, err := h.queries.ListTailoredCVLabelsBySessions(c.Context(), db.ListTailoredCVLabelsBySessionsParams{
			UserID: userID, SessionIds: sessionRefs,
		})
		if err != nil {
			return nil, err
		}
		bySession := make(map[string]string, len(cvs))
		for _, cv := range cvs {
			bySession[cv.AgentSessionID.String] = jobLabel(cv.JobTitle, cv.JobSlug)
		}
		for ref, session := range sessionByRef {
			if label, ok := bySession[session]; ok {
				subjects[subjectKey(string(plan.FeatureTailor), ref)] = label
			}
		}
	}
	return subjects, nil
}

// subjectKey namespaces a resolved subject by its feature, so two references that look alike
// under different features do not collide in the map.
func subjectKey(feature, ref string) string { return feature + ":" + ref }

// jobLabel prefers a job's title, falling back to its public slug when the title is blank.
func jobLabel(title, slug string) string {
	if title != "" {
		return title
	}
	return slug
}
