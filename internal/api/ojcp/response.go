package ojcp

// The response envelopes every OJCP tool answers in. Each carries `ojcp_version` so an
// agent can tell which spec produced what it received, and each is Finalize()d rather than
// built field-by-field at the call site: the version and the derived counts are exactly the
// fields a second caller would eventually forget.

// SearchJobsResponse is the `search_jobs` answer.
type SearchJobsResponse struct {
	OJCPVersion  string `json:"ojcp_version"`
	Query        string `json:"query"`
	TotalResults int    `json:"total_results"`
	// Returned is derived from Jobs by Finalize — it is the one number that can disagree
	// with what is actually in the envelope.
	Returned int          `json:"returned"`
	Offset   int          `json:"offset"`
	Jobs     []JobPosting `json:"jobs"`
	// IgnoredParams names the input fields this provider could not honour. OJCP has no
	// field for it and this is an EXTENSION, which its extensibility rule permits.
	//
	// It exists because of the house rule an endpoint here must follow: an answer that
	// WIDENS because a parameter was not understood has to say so. An agent that asked for
	// jobs within 20 miles and silently received the whole catalogue cannot tell the
	// narrowing never happened, and neither can the person reading what it found.
	IgnoredParams []string `json:"ignored_params,omitempty"`
}

// Finalize stamps the version and derives what can be derived. `jobs` is required by the
// schema, so an empty page serialises as [] rather than null: a null there reads as "this
// provider is broken", while [] is a real answer meaning nothing matched.
func (r SearchJobsResponse) Finalize() SearchJobsResponse {
	r.OJCPVersion = Version
	if r.Jobs == nil {
		r.Jobs = []JobPosting{}
	}
	r.Returned = len(r.Jobs)
	return r
}

// JobDetailResponse is the `get_job_detail` answer.
type JobDetailResponse struct {
	OJCPVersion string     `json:"ojcp_version"`
	Job         JobPosting `json:"job"`
}

func (r JobDetailResponse) Finalize() JobDetailResponse {
	r.OJCPVersion = Version
	return r
}

// EmployerContextResponse is the `get_employer_context` answer: what we know about an
// employer beyond one posting.
type EmployerContextResponse struct {
	OJCPVersion string            `json:"ojcp_version"`
	EmployerID  string            `json:"employer_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Industries  []string          `json:"industries,omitempty"`
	HQLocation  *EmployerLocation `json:"hq_location,omitempty"`
	// OpenRolesCount is how many of this employer's postings are open right now — the one
	// figure an agent cannot derive from a single posting, and the reason to call this tool
	// at all.
	OpenRolesCount int `json:"open_roles_count,omitempty"`
}

// EmployerLocation is the standard's headquarters block. Its `state` is a real province
// field, unlike a posting's addressRegion, but we hold no such facet and leave it unset
// rather than filling it from our macro-regions.
type EmployerLocation struct {
	City    string `json:"city,omitempty"`
	State   string `json:"state,omitempty"`
	Country string `json:"country,omitempty"`
}

func (r EmployerContextResponse) Finalize() EmployerContextResponse {
	r.OJCPVersion = Version
	return r
}

// Error codes, from the CLOSED enum in the standard's own error schema. An agent branches
// on these, so a code of our own invention is not a smaller answer — it is an unreadable
// one, and the schema rejects the whole envelope.
const (
	ErrorJobNotFound      = "job_not_found"
	ErrorEmployerNotFound = "employer_not_found"
	ErrorInvalidRequest   = "invalid_request"
	ErrorProviderError    = "provider_error"
	ErrorRateLimited      = "rate_limited"
)

// ErrorCodes is every code this package may emit. It exists so a caller mapping codes onto
// something else — an HTTP status, a JSON-RPC code — can be held to covering all of them by
// a test that walks THIS list rather than a copy of it written beside the mapping.
func ErrorCodes() []string {
	return []string{
		ErrorJobNotFound, ErrorEmployerNotFound, ErrorInvalidRequest,
		ErrorProviderError, ErrorRateLimited,
	}
}

// ErrorResponse is the envelope both transports render a failure in — as an HTTP body over
// REST, and inside a JSON-RPC error's `data` over MCP.
//
// The fields are FLAT, and `error_code` is the standard's own name for the discriminator.
// An earlier version nested them under an `error` object with a `code`, which no part of
// the schema describes — the oracle never saw it because `schemaErrorResponse` was declared
// and never used, which is what a switched-off check looks like from the inside.
type ErrorResponse struct {
	OJCPVersion string `json:"ojcp_version"`
	ErrorCode   string `json:"error_code"`
	Message     string `json:"message"`
	// RetryAfterSeconds is required by the schema alongside a rate-limit refusal and
	// meaningless otherwise, so it is omitted rather than sent as zero.
	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`
}

// NewError builds the error envelope. Both transports go through it so a failure cannot be
// described one way over REST and another over MCP.
func NewError(code, message string) ErrorResponse {
	return ErrorResponse{OJCPVersion: Version, ErrorCode: code, Message: message}
}

// NewRateLimitError is the one refusal with a required companion field: the schema makes
// `retry_after_seconds` mandatory alongside `rate_limited`, because an agent that is told
// to slow down and not told for how long can only guess — and a guessing agent retries.
//
// It is a separate constructor rather than an argument on NewError so the requirement
// cannot be forgotten at a call site: there is no way to say `rate_limited` without saying
// how long.
func NewRateLimitError(retryAfterSeconds int) ErrorResponse {
	return ErrorResponse{
		OJCPVersion:       Version,
		ErrorCode:         ErrorRateLimited,
		Message:           "too many requests; see retry_after_seconds",
		RetryAfterSeconds: retryAfterSeconds,
	}
}
