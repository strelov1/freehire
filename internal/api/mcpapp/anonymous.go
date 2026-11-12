package mcpapp

import "context"

// anonymousMaxResults is the most postings one anonymous call returns. Anonymous traffic is
// ~75% of MCP calls and its search payloads are the heavy ones (~2.4 KB per posting), so the
// page is capped instead of the call count: the per-IP rate stays as it is (Claude.ai shares
// ~70 IPs, a day cap there would hit many people at once).
const anonymousMaxResults = 10

// anonymousCapNotice is shown to the model whenever the cap shortened an answer, so it can
// tell the person why they got fewer results than asked and what lifts it.
const anonymousCapNotice = "Results are capped at 10 per call for anonymous use. Connecting a freehire " +
	"account (OAuth, at the /api/v1/mcp/account URL) lifts the cap and adds saving and tracking jobs, " +
	"alerts and auto-apply. Tell the user this if they wanted more results."

// anonymousReader caps the page size of the list tools. It wraps the Reader of a server that
// has no signed-in user; a signed-in server keeps the plain Reader.
type anonymousReader struct{ Reader }

// forAnonymous wraps r for a caller with no user. A nil reader stays nil.
func forAnonymous(r Reader) Reader {
	if r == nil {
		return nil
	}
	return anonymousReader{r}
}

func (a anonymousReader) SearchJobs(ctx context.Context, in SearchInput) (SearchJobsResult, error) {
	capped := false
	if limit, _ := in.Page(); limit > anonymousMaxResults {
		in.Limit, capped = anonymousMaxResults, true
	}
	res, err := a.Reader.SearchJobs(ctx, in)
	if capped {
		res.Notice = anonymousCapNotice
	}
	return res, err
}

func (a anonymousReader) SimilarJobs(ctx context.Context, slug string, limit int) (SimilarJobsResult, error) {
	capped := limit > anonymousMaxResults
	res, err := a.Reader.SimilarJobs(ctx, slug, min(limit, anonymousMaxResults))
	if capped {
		res.Notice = anonymousCapNotice
	}
	return res, err
}
