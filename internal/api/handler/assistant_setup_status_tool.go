package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// setupFacts is what get_setup_status reads, all of it existing state. The *Known flags are
// false where the deployment wires no source for a step, which leaves the step out rather
// than reporting a state nobody read.
type setupFacts struct {
	cvStorage bool
	cvPresent bool
	cvReading string // a resume.ExtractStatus* value, "" when none is recorded

	specializations, skills, seniorities, workModes bool

	alertsKnown  bool
	activeAlerts int

	tierKnown bool
	tier      plan.Tier

	agentsKnown    bool
	agentsMayApply bool
}

type setupStatus struct {
	// Complete is the core setup done — a CV, preferences, an alert. The auto-apply steps are
	// optional and do not count.
	Complete bool        `json:"complete"`
	Steps    []setupStep `json:"steps"`
	NextStep setupNext   `json:"next_step"`
}

type setupStep struct {
	Name     string   `json:"name"`
	Done     bool     `json:"done"`
	Optional bool     `json:"optional,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	Missing  []string `json:"missing,omitempty"`
}

// setupNext is the ONE thing to do now: a tool to call, or a page to send the person to.
type setupNext struct {
	Step string `json:"step"`
	Do   string `json:"do"`
	Tool string `json:"tool,omitempty"`
	URL  string `json:"url,omitempty"`
}

// setupStatusTool reports what this account is missing to get value from freehire, and the
// next step. Read-only and derived on every call; nothing is stored about setup itself.
func (h *assistantHandlers) setupStatusTool(origin string) assistant.Tool {
	return assistant.Tool{
		Name:       "get_setup_status",
		Title:      "Check my freehire setup",
		Effect:     assistant.EffectReads,
		Idempotent: true,
		Description: "Report what this account still needs to get value from freehire: a CV (and whether " +
			"reading it has finished), job-search preferences, an active job alert, and — only needed " +
			"for applications prepared on the person's behalf — a paid plan and the switch letting " +
			"connected agents apply. Returns each step with done or not, which preferences are missing, " +
			"and next_step: the one thing to do now, as a tool name or a page URL to send the person to. " +
			"Takes no arguments.",
		Schema: assistant.ClosedEmptySchema,
		Run: func(ctx context.Context, userID int64, _ json.RawMessage) (any, error) {
			f, err := h.setupFacts(ctx, userID)
			if err != nil {
				return nil, err
			}
			return buildSetupStatus(f, origin), nil
		},
	}
}

// setupFacts reads each step's state. A failed read fails the call: a step reported as not
// done because its read failed would send the person to redo something they did.
func (h *assistantHandlers) setupFacts(ctx context.Context, userID int64) (setupFacts, error) {
	if h.profile == nil || h.profile.userProfile == nil || h.resume == nil {
		return setupFacts{}, unavailable("the account setup")
	}
	var f setupFacts
	if store := h.resume.resume; store.Enabled() {
		f.cvStorage = true
		meta, err := store.Status(ctx, userID)
		if err != nil {
			return setupFacts{}, err
		}
		f.cvPresent = meta.Present
		if f.cvPresent {
			row, err := store.StructuredRow(ctx, userID)
			if err != nil {
				return setupFacts{}, err
			}
			f.cvReading = resume.ResolveExtractStatus(row).Status
		}
	}

	p, err := h.profile.userProfile.Get(ctx, userID)
	if err != nil && !errors.Is(err, userprofile.ErrNotFound) {
		return setupFacts{}, err
	}
	f.specializations, f.skills, f.seniorities = len(p.Specializations) > 0, len(p.Skills) > 0, len(p.Seniorities) > 0
	if len(p.LocationPreferences) > 0 {
		var loc userprofile.LocationPreferences
		if json.Unmarshal(p.LocationPreferences, &loc) == nil {
			f.workModes = len(loc.WorkModes) > 0
		}
	}

	if h.queries != nil {
		subs, err := h.subscriptions().List(ctx, userID)
		if err != nil {
			return setupFacts{}, err
		}
		f.alertsKnown = true
		for _, s := range subs {
			if s.Active {
				f.activeAlerts++
			}
		}
		if f.agentsMayApply, err = h.queries.GetAgentsMayApply(ctx, userID); err != nil {
			return setupFacts{}, err
		}
		f.agentsKnown = true
	}
	if h.plans != nil {
		if f.tier, err = h.plans.Tier(ctx, userID); err != nil {
			return setupFacts{}, err
		}
		f.tierKnown = true
	}
	return f, nil
}

// buildSetupStatus orders the steps the way a person gets value from them — a CV, the
// preferences it suggests, an alert, then the optional auto-apply prerequisites — and names
// the first core one not done.
func buildSetupStatus(f setupFacts, origin string) setupStatus {
	page := func(path string) string { return strings.TrimSuffix(origin, "/") + path }
	type candidate struct {
		step setupStep
		next setupNext
	}
	var steps []candidate

	cv := candidate{step: setupStep{Name: "cv", Done: f.cvPresent}}
	switch {
	case !f.cvPresent && f.cvStorage:
		cv.next = setupNext{Tool: "add_cv", URL: page("/my/profile"), Do: "Ask the person for their CV. " +
			"If they paste it or give you a CV file, pass it verbatim with add_cv. If they have no CV, " +
			"send them to the URL to upload or write one; never compose one for them."}
	case !f.cvPresent:
		cv.next = setupNext{URL: page("/my/profile"), Do: "Ask the person to upload their CV on their profile page."}
	case f.cvReading == resume.ExtractStatusFailed:
		cv.step.Done, cv.step.Detail = false, "unreadable"
		cv.next = setupNext{URL: page("/my/profile"), Do: "freehire could not read the person's CV. " +
			"Ask them to retry reading it, or upload a text-based PDF, on their profile page."}
	case f.cvReading == resume.ExtractStatusPending:
		cv.step.Detail = "reading"
	}
	steps = append(steps, cv)

	var missing []string
	for _, m := range []struct {
		name string
		set  bool
	}{{"specializations", f.specializations}, {"skills", f.skills}, {"seniorities", f.seniorities}, {"work_modes", f.workModes}} {
		if !m.set {
			missing = append(missing, m.name)
		}
	}
	steps = append(steps, candidate{
		step: setupStep{Name: "preferences", Done: f.specializations && f.skills, Missing: missing},
		next: setupNext{Tool: "update_profile", Do: "Read the profile with get_profile, propose values for " +
			"what is missing (from the CV where there is one), ask the person to confirm each, and save " +
			"only the confirmed ones with update_profile."},
	})

	if f.alertsKnown {
		steps = append(steps, candidate{
			step: setupStep{Name: "alerts", Done: f.activeAlerts > 0},
			next: setupNext{Tool: "subscribe", Do: "Offer a job alert on a search built from their profile, " +
				"and create it with subscribe once they agree."},
		})
	}
	if f.tierKnown {
		steps = append(steps, candidate{
			step: setupStep{Name: "paid_plan", Done: f.tier.Paid(), Optional: true, Detail: string(f.tier)},
			next: setupNext{URL: page("/pricing"), Do: "Only if the person wants applications prepared for " +
				"them: that needs a paid plan, chosen at the URL."},
		})
	}
	if f.agentsKnown {
		steps = append(steps, candidate{
			step: setupStep{Name: "agents_may_apply", Done: f.agentsMayApply, Optional: true},
			next: setupNext{URL: page("/my/integrations/api-keys"), Do: "Only if the person wants applications " +
				"prepared for them: they turn on \"Let connected agents apply for me\" at the URL."},
		})
	}

	// Only a core step is ever the next one: the optional ones stay visible in steps, and once
	// the core is done the person is pointed at a search rather than at a plan.
	out := setupStatus{Complete: true, NextStep: setupNext{Step: "done", Tool: "search_jobs",
		Do: "Setup is complete. Search for jobs that fit the person's profile."}}
	for _, c := range steps {
		out.Steps = append(out.Steps, c.step)
		if c.step.Done || c.step.Optional {
			continue
		}
		if out.Complete {
			out.NextStep = c.next
			out.NextStep.Step = c.step.Name
		}
		out.Complete = false
	}
	return out
}
