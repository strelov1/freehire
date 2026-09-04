package main

import (
	"slices"

	"github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/jobderive"
)

// The three rules that make a job a deletion target. The name is recorded on every
// archive row, so a rule that turns out to be too broad can be audited — and undone in
// judgement, if not in data — on its own.
const (
	// ruleTitle: the non-tech title dictionary recognises the posting. Enforced at
	// ingest too, which is what makes it self-sufficient: the same term that deletes
	// the row also stops the next crawl from re-admitting it.
	ruleTitle = "title"
	// ruleBusiness: a business role — sales, recruiting, finance — at a company that
	// has never posted anything technical. The role itself is in scope at an IT
	// company; the company is what disqualifies it.
	ruleBusiness = "business_at_nontech_company"
	// ruleUnknown: a job no dictionary could place, at a company that has shown no
	// technical signal of any kind, not even a tagged skill.
	ruleUnknown = "unknown_at_empty_company"
)

// candidate is the part of a job the rule reads. Everything here is a stored column;
// the rule derives its own signals from them rather than trusting a stored is_tech,
// because the campaign edits the dictionaries between runs and a stale label would
// decide a permanent deletion.
type candidate struct {
	CompanySlug string
	Title       string
	Category    string
	// IsTech is the stored tri-state, used only to tell "no dictionary placed this"
	// (nil) from "a dictionary placed it as non-technical" (false). The positive case
	// is re-derived, never read from here.
	IsTech *bool
}

// evidence is what a company has ever shown, across its entire history.
type evidence struct {
	anyTech   bool
	anySkills bool
}

// matchRule reports which rule makes a job a deletion target, if any. The empty result
// means keep.
//
// Two booleans carry the safety design. knownProvider gates everything: a source with
// no boards at all is written outside the ingest pipeline and no crawl restores it.
// boardCrawled — whether the posting's board is still in the source files — is then read
// in OPPOSITE directions by the two families of rule.
//
// The title rule requires it TRUE. A listed board is by definition re-crawlable, which
// is what makes a title deletion recoverable: withdraw an over-broad dictionary term and
// the postings return on the next pass. A posting whose board is not listed — a
// link-source import, a moderator row, a board struck long ago — is unrecoverable, so
// the rule that is otherwise the safest becomes the most dangerous and must not apply.
//
// The company-scoped rules require it FALSE. They have no counterpart at crawl time,
// so what they remove comes back within the hour unless the board is gone.
//
// Between them sits the technical veto, for the same reason it vetoes the ingest filter:
// the non-tech dictionary matches anywhere in a title and was written assuming the tech
// check runs first, so "Backend Engineer — Teller Systems" must survive "teller".
func matchRule(c candidate, ev evidence, knownProvider, boardCrawled bool) (string, bool) {
	// A source that is not a crawled board platform is out of reach of every rule. It
	// would otherwise pass the company-scoped rules for free: they ask for an absent
	// board, and a source with no boards has nothing but absent ones.
	if !knownProvider {
		return "", false
	}
	techEvidence := jobderive.TechEvidence(c.Category, c.Title)

	if classify.ConfirmedNonTech(c.Title, techEvidence) {
		if !boardCrawled {
			return "", false
		}
		return ruleTitle, true
	}
	if techEvidence || boardCrawled {
		return "", false
	}
	// The company-scoped rules rest on a company's history, so a posting with no
	// company has none to rest on. Without this they would all pool under the empty
	// slug and be decided together.
	if c.CompanySlug == "" {
		return "", false
	}
	if !ev.anyTech && isBusinessCategory(c.Category) {
		return ruleBusiness, true
	}
	if !ev.anyTech && !ev.anySkills && c.IsTech == nil {
		return ruleUnknown, true
	}
	return "", false
}

// isBusinessCategory reports whether a category names the back-office and go-to-market
// work ruleBusiness deletes at a company with no technical history. It is
// vocab.NonTechCategories minus vocab.NonTechCraftCategories: those are non-technical
// because an IT job board is not where a mechanical draughtsman or a process engineer
// looks for work, not because the posting is a business role at a software employer.
// Deleting them here would take out an engineering employer's whole catalogue the
// moment its board is retired — the same defect the ConfirmedNonTech veto fixes on the
// title path, which this rule does not go through.
//
// The subtraction reads a vocabulary set rather than naming a category inline. It was
// one inline name until a second craft category arrived, and that shape had already
// failed once in principle: a category joins NonTechCategories in internal/dict/vocab,
// by someone with no reason to open cmd/prune, and becomes hard-deletable in silence.
func isBusinessCategory(category string) bool {
	if slices.Contains(vocab.NonTechCraftCategories, category) {
		return false
	}
	return slices.Contains(vocab.NonTechCategories, category)
}

// companyScoped reports whether a rule depends on the company's history rather than on
// the posting alone. It matters because boards are re-crawled hourly on an unchanged
// dedup key: the title rule is mirrored by the ingest filter, so what it deletes stays
// deleted, but nothing at crawl time knows a company's bucket. A company-scoped
// deletion whose board is still live in the catalog undoes itself within the hour,
// which is why the worker refuses to run one without retiring the board in the same
// step.
func companyScoped(rule string) bool {
	return rule == ruleBusiness || rule == ruleUnknown
}
