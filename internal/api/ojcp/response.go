package ojcp

// The response envelopes every OJCP tool answers in. Each carries `ojcp_version` and is
// Finalize()d so callers cannot forget the version or the derived counts.

// SearchJobsResponse is the `search_jobs` answer.
type SearchJobsResponse struct {
	OJCPVersion  string `json:"ojcp_version"`
	Query        string `json:"query"`
	TotalResults int    `json:"total_results"`
	// Returned is derived from Jobs by Finalize.
	Returned int          `json:"returned"`
	Offset   int          `json:"offset"`
	Jobs     []JobPosting `json:"jobs"`
	// IgnoredParams names the input fields this provider could not honour. An extension
	// (OJCP permits them): an answer that widens because a parameter was not understood
	// has to say so.
	IgnoredParams []string `json:"ignored_params,omitempty"`
	// TotalIsLowerBound marks TotalResults as "at least this many" rather than a count.
	// An extension, for the reason IgnoredParams is one.
	TotalIsLowerBound bool `json:"total_is_lower_bound,omitempty"`
}

// Finalize stamps the version and derives the counts. `jobs` is required by the schema, so
// an empty page serialises as [] rather than null.
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
	// OpenRolesCount is how many of this employer's postings are open right now.
	OpenRolesCount int `json:"open_roles_count,omitempty"`
}

// EmployerLocation is the standard's headquarters block. `state` stays unset: we hold no
// province facet and must not fill it from our macro-regions.
type EmployerLocation struct {
	City    string `json:"city,omitempty"`
	State   string `json:"state,omitempty"`
	Country string `json:"country,omitempty"`
}

func (r EmployerContextResponse) Finalize() EmployerContextResponse {
	r.OJCPVersion = Version
	return r
}

// Error codes, from the closed enum in the standard's error schema; an invented code makes
// the schema reject the whole envelope.
const (
	ErrorJobNotFound      = "job_not_found"
	ErrorEmployerNotFound = "employer_not_found"
	ErrorInvalidRequest   = "invalid_request"
	ErrorProviderError    = "provider_error"
)

// ErrorCodes is every code this package may emit, so a test can hold a caller's mapping
// (HTTP status, JSON-RPC code) to covering all of them.
func ErrorCodes() []string {
	return []string{
		ErrorJobNotFound, ErrorEmployerNotFound, ErrorInvalidRequest,
		ErrorProviderError,
	}
}

// ErrorResponse is the envelope both transports render a failure in: an HTTP body over
// REST, a JSON-RPC error's `data` over MCP. The fields are flat, and `error_code` is the
// standard's own name for the discriminator.
type ErrorResponse struct {
	OJCPVersion string `json:"ojcp_version"`
	ErrorCode   string `json:"error_code"`
	Message     string `json:"message"`
}

// NewError builds the error envelope; both transports use it.
func NewError(code, message string) ErrorResponse {
	return ErrorResponse{OJCPVersion: Version, ErrorCode: code, Message: message}
}
