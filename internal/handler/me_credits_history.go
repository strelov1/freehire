package handler

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// creditHistoryLimit bounds the transaction history to a recent window. The ledger for a
// free-tier user is small; a cursor API can be added behind the same endpoint if it ever grows.
const creditHistoryLimit = 100

// creditHistoryEntry is the display-ready shape of one credit-ledger row on the Credits page:
// a signed delta with a human label (and, for a metered debit, the job it named as subtitle).
type creditHistoryEntry struct {
	Kind      string    `json:"kind"`
	Delta     int32     `json:"delta"`
	Label     string    `json:"label"`
	Subtitle  string    `json:"subtitle,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// creditEntryLabel returns the display label and optional subtitle for a ledger row. kind is
// the ledger kind (grant|debit|reward|purchase); feature is the debit feature (match|tailor)
// or ""; subject is the resolved job title for a debit, or "" when the job is unknown/deleted.
func creditEntryLabel(kind, feature, subject string) (label, subtitle string) {
	switch kind {
	case "grant":
		return "Monthly grant", ""
	case "reward":
		return "Board contribution", ""
	case "purchase":
		return "Credit purchase", ""
	case "debit":
		switch feature {
		case "match":
			return "Match analysis", subject
		case "tailor":
			return "CV tailoring", subject
		default:
			return "Credit used", ""
		}
	default:
		return "Credit adjustment", ""
	}
}

// GetMyCreditsHistory returns the caller's credit-ledger entries newest first, each labelled
// for display. Cookie or API key; never calls the LLM. Debit refs are resolved in two batch
// lookups (match → job title, tailor → the tailored CV's job) so the list reads in plain terms.
func (a *API) GetMyCreditsHistory(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := a.queries.ListCreditLedger(c.Context(), db.ListCreditLedgerParams{UserID: userID, Limit: creditHistoryLimit})
	if err != nil {
		return err
	}
	subjects, err := a.resolveDebitSubjects(c, rows)
	if err != nil {
		return err
	}

	entries := make([]creditHistoryEntry, 0, len(rows))
	for _, r := range rows {
		feature := r.Feature.String // "" when NULL
		subject := ""
		if r.Kind == "debit" {
			subject = subjects[subjectKey(feature, r.Ref.String)]
		}
		label, subtitle := creditEntryLabel(r.Kind, feature, subject)
		entries = append(entries, creditHistoryEntry{
			Kind:      r.Kind,
			Delta:     r.Delta,
			Label:     label,
			Subtitle:  subtitle,
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return c.JSON(fiber.Map{"data": entries})
}

// resolveDebitSubjects batch-resolves the job titles named by the debit rows, keyed by
// subjectKey(feature, ref): match debits name a job id directly, tailor debits name a tailored
// CV id whose target job supplies the title. The feature is part of the key so a job id and a
// CV id that happen to share a numeric value never collide. A ref whose subject was deleted is
// simply absent, so the caller falls back to a generic label.
func (a *API) resolveDebitSubjects(c *fiber.Ctx, rows []db.ListCreditLedgerRow) (map[string]string, error) {
	var jobRefs, cvRefs []int64
	for _, r := range rows {
		if r.Kind != "debit" || !r.Ref.Valid {
			continue
		}
		id, err := strconv.ParseInt(r.Ref.String, 10, 64)
		if err != nil {
			continue // a non-numeric ref never resolves; the generic label stands
		}
		switch r.Feature.String {
		case "match":
			jobRefs = append(jobRefs, id)
		case "tailor":
			cvRefs = append(cvRefs, id)
		}
	}

	subjects := make(map[string]string)
	if len(jobRefs) > 0 {
		jobs, err := a.queries.ListJobLabelsByIDs(c.Context(), jobRefs)
		if err != nil {
			return nil, err
		}
		for _, j := range jobs {
			subjects[subjectKey("match", strconv.FormatInt(j.ID, 10))] = jobLabel(j.Title, j.PublicSlug)
		}
	}
	if len(cvRefs) > 0 {
		cvs, err := a.queries.ListTailoredCVLabelsByIDs(c.Context(), cvRefs)
		if err != nil {
			return nil, err
		}
		for _, cv := range cvs {
			subjects[subjectKey("tailor", strconv.FormatInt(cv.ID, 10))] = jobLabel(cv.JobTitle, cv.JobSlug)
		}
	}
	return subjects, nil
}

// subjectKey namespaces a resolved subject by its debit feature so a match's job id and a
// tailor's CV id with the same numeric value do not collide in the subjects map.
func subjectKey(feature, ref string) string { return feature + ":" + ref }

// jobLabel prefers a job's title, falling back to its public slug when the title is blank.
func jobLabel(title, slug string) string {
	if title != "" {
		return title
	}
	return slug
}
