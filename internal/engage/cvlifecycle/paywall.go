package cvlifecycle

import "fmt"

// attempt words what a refused feature was, to finish "You wanted to ...". generic stands
// alone; withJob has one %s for the posting and is used when the hit named one.
type attempt struct {
	generic string
	withJob string
}

// attempts is keyed by the label the paid-AI gate records: a plan feature for a metered
// route, a fixed name for the three that draw on no allowance (see handler.paid_ai.go). A
// label missing here still gets a letter, with fallbackAttempt, so a route added to the gate
// before its words are written here is never a silent hole.
var attempts = map[string]attempt{
	"tailor":       {"tailor your CV", "tailor your CV for %s"},
	"match":        {"see how well you fit a posting", "see how well you fit %s"},
	"assistant":    {"talk to the AI assistant", "talk through %s with the AI assistant"},
	"dictation":    {"dictate to the AI assistant", ""},
	"cover-letter": {"draft a cover letter", "draft a cover letter for %s"},
	"auto-apply":   {"apply with auto-apply", "apply to %s with auto-apply"},
	"ats_review":   {"get the AI review of your CV", ""},
	"mail_recall":  {"pull the emails about an application into your tracker", "pull the emails about %s into your tracker"},
	"autofill":     {"have the agent fill in an application form", "have the agent fill in the application for %s"},
}

var fallbackAttempt = attempt{generic: "use one of the AI features"}

// posting names the vacancy a hit concerned: "Title at Company", or the title alone. Empty
// when the hit carried no job.
func (p Paywall) posting() string {
	switch {
	case p.JobTitle == "":
		return ""
	case p.Company == "":
		return p.JobTitle
	}
	return p.JobTitle + " at " + p.Company
}

// described returns what the account tried, in the generic form for the subject and, when
// the hit named a posting the feature is about, in the specific form for the body.
func (p Paywall) described() (generic, specific string) {
	a, ok := attempts[p.Feature]
	if !ok {
		a = fallbackAttempt
	}
	if posting := p.posting(); posting != "" && a.withJob != "" {
		return a.generic, fmt.Sprintf(a.withJob, posting)
	}
	return a.generic, a.generic
}
