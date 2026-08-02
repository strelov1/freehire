package applyform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ErrPostingGone marks a form the platform will never serve because the posting is no
// longer on the board. It is not a failure to retry: the catalogue simply still holds a
// posting the employer took down, and the unseen sweep closes it within 48h.
//
// The distinction earns its keep at scale. Across a backlog of a quarter of a million
// postings there are thousands of these; retried, each burns three requests and then
// dead-letters, and a queue steadily accumulating dead letters is indistinguishable from
// one that is genuinely broken.
var ErrPostingGone = errors.New("posting is gone")

// statusCoder is any error carrying an HTTP status. Declared here rather than imported
// because internal/sources imports THIS package (for the form its Recruitee adapter
// yields), so the dependency cannot run both ways; sources.StatusError satisfies it.
type statusCoder interface{ StatusCode() int }

// asGone converts a not-found response into ErrPostingGone and leaves everything else
// retryable. A 404 is the platform stating the posting does not exist; a 429 or a 5xx is
// the platform declining to answer right now, which is a different thing entirely.
func asGone(err error) error {
	var sc statusCoder
	if errors.As(err, &sc) && sc.StatusCode() == http.StatusNotFound {
		return fmt.Errorf("%w: %v", ErrPostingGone, err)
	}
	return err
}

// Transport is the narrow HTTP role a fetcher needs: a JSON GET for Greenhouse's REST
// endpoint and a JSON POST for Ashby's GraphQL one. It is declared here rather than
// imported from internal/sources because the dependency runs the other way — sources
// imports THIS package to attach the form Recruitee's listing already carries. The real
// sources.Client satisfies it structurally, so the worker passes the same client the crawl
// uses.
type Transport interface {
	GetJSON(ctx context.Context, url string, v any) error
	PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error
}

// Fetcher retrieves one posting's application form from the platform that published it.
// board and postingID are the two halves of the stored external_id.
type Fetcher interface {
	Fetch(ctx context.Context, board, postingID string) (Form, error)
}

// NeedsRequestCapture reports whether a provider's application form has to be fetched per
// posting, which is what the ingest write path uses to decide whether to queue a capture.
//
// False covers two very different cases on purpose, because the write path treats them the
// same: recruitee, whose form arrives free with the crawl and is written directly, and
// every other provider, whose form cannot be read at all. Queueing either would fill the
// outbox with work nothing can drain.
func NeedsRequestCapture(provider string) bool {
	_, ok := fetcherFor[provider]
	return ok
}

// fetcherFor is the single registry behind both the enqueue gate and the worker's drain.
// Keeping them on one map is what stops the two from drifting into a queue full of
// undrainable work, and a test holds them to it.
var fetcherFor = map[string]func(Transport) Fetcher{
	"greenhouse": func(t Transport) Fetcher { return greenhouseFetcher{http: t} },
	"ashby":      func(t Transport) Fetcher { return ashbyFetcher{http: t} },
}

// Fetchers builds the per-provider fetcher registry over one transport.
func Fetchers(t Transport) map[string]Fetcher {
	out := make(map[string]Fetcher, len(fetcherFor))
	for provider, build := range fetcherFor {
		out[provider] = build(t)
	}
	return out
}

// splitBoardPosting splits a stored external_id back into the board and the platform's own
// posting id. sources.NamespaceExternalID composes it as "board:id" with the board first,
// so the split is on the FIRST colon: a board name is an ATS slug and carries none, while
// a posting id might.
func splitBoardPosting(externalID string) (board, posting string, ok bool) {
	board, posting, ok = strings.Cut(externalID, ":")
	if !ok || board == "" || posting == "" {
		return "", "", false
	}
	return board, posting, true
}

// decodeBoardName undoes the percent-encoding a board name carries.
//
// Board names are stored as they appear in the board file, where they are already encoded
// for the URL PATH the crawl adapter builds — Greenhouse and Ashby's posting API both take
// the board as a path segment, so "stony%20creek%20homes" is correct there. Ashby's GraphQL
// takes the organization as a VARIABLE, and an encoded name is simply a wrong name: the API
// answers 200 with a null posting, so every board whose name carries a space would fail
// every capture forever. A name that is not valid encoding is returned as it stands —
// refusing to fetch would be worse than trying the literal.
func decodeBoardName(board string) string {
	decoded, err := url.PathUnescape(board)
	if err != nil {
		return board
	}
	return decoded
}

// greenhouseBaseURL is the job-board API. One host serves EU-hosted boards too, so unlike
// Lever there is no regional base URL to pick between.
const greenhouseBaseURL = "https://boards-api.greenhouse.io/v1/boards"

type greenhouseFetcher struct{ http Transport }

func (g greenhouseFetcher) Fetch(ctx context.Context, board, postingID string) (Form, error) {
	// questions=true is the point of the request: the board listing ignores the parameter
	// and the per-posting endpoint without it answers 200 with no form at all.
	url := fmt.Sprintf("%s/%s/jobs/%s?questions=true", greenhouseBaseURL, board, postingID)

	var job GreenhouseJob
	if err := g.http.GetJSON(ctx, url, &job); err != nil {
		return Form{}, fmt.Errorf("greenhouse: fetch form for %s/%s: %w", board, postingID, asGone(err))
	}
	return FromGreenhouse(job), nil
}

// ashbyGraphQLURL is the job board's unauthenticated GraphQL endpoint. The op= parameter is
// the operation name repeated in the query string, which is how their gateway routes.
const ashbyGraphQLURL = "https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobPosting"

// ashbyQuery asks for the posting's form. `field` is requested as a BARE SCALAR because
// Ashby types it `JSON!`: a selection set on it fails the entire query with a validation
// error, not just that field, so every capture would break at once.
const ashbyQuery = `query ApiJobPosting($organizationHostedJobsPageName: String!, $jobPostingId: String!) {
  jobPosting(organizationHostedJobsPageName: $organizationHostedJobsPageName, jobPostingId: $jobPostingId) {
    applicationForm {
      sections {
        title
        isHidden
        fieldEntries { id isRequired isHidden field }
      }
    }
  }
}`

type ashbyFetcher struct{ http Transport }

func (a ashbyFetcher) Fetch(ctx context.Context, board, postingID string) (Form, error) {
	body := map[string]any{
		"operationName": "ApiJobPosting",
		"variables": map[string]any{
			"organizationHostedJobsPageName": decodeBoardName(board),
			"jobPostingId":                   postingID,
		},
		"query": ashbyQuery,
	}

	var resp struct {
		Data struct {
			JobPosting *struct {
				ApplicationForm AshbyApplicationForm `json:"applicationForm"`
			} `json:"jobPosting"`
		} `json:"data"`
	}
	headers := map[string]string{"content-type": "application/json"}
	if err := a.http.PostJSONWithHeaders(ctx, ashbyGraphQLURL, headers, body, &resp); err != nil {
		return Form{}, fmt.Errorf("ashby: fetch form for %s/%s: %w", board, postingID, asGone(err))
	}
	// A posting Ashby does not have answers 200 with a null jobPosting rather than an
	// error, so an absent posting would otherwise be captured as an empty form — which
	// reads as "this employer asks nothing" and is a lie.
	if resp.Data.JobPosting == nil {
		return Form{}, fmt.Errorf("ashby: no posting %s/%s: %w", board, postingID, ErrPostingGone)
	}
	return FromAshby(resp.Data.JobPosting.ApplicationForm), nil
}
