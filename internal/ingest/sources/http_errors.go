package sources

import (
	"fmt"
)

// BodyTooLargeError is a response whose body ran past the size cap and was truncated mid-stream.
// It is NOT transient: the same board returns the same oversized body every run, so callers must
// not treat it as a fetch that might recover. A bare io.LimitReader would hand the decoder a
// truncated stream indistinguishable from a dropped connection.
type BodyTooLargeError struct {
	URL   string
	Limit int64
}

func (e *BodyTooLargeError) Error() string {
	return fmt.Sprintf("sources: GET %s: body exceeds %d bytes", e.URL, e.Limit)
}

// StatusError is a non-2xx HTTP response from the sources client. Callers that need
// to branch on the status code (e.g. eightfold's rate-limit backoff) match it with
// errors.As instead of scraping the message. Its Error() renders the same
// "sources: <METHOD> <url>: status <n>" text the client has always produced, so any
// remaining string-based check keeps working.
type StatusError struct {
	Method string
	Code   int
	URL    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("sources: %s %s: status %d", e.Method, e.URL, e.Code)
}

// StatusCode reports the response status. It exists so a package this one IMPORTS can
// still branch on the code: internal/ingest/applyform is imported by the Recruitee adapter, so
// it cannot import sources back, and a one-method interface it declares locally is
// satisfied by this without either package naming the other.
func (e *StatusError) StatusCode() int { return e.Code }

// ChallengeError is an AWS-WAF Challenge response — a request the WAF answered with an
// interactive JS proof-of-work shell instead of the real content (marked by the
// "x-amzn-waf-action: challenge" header, served with HTTP 202). It is NOT a transient
// failure: a fixed backoff will not clear it, and retrying only deepens the per-IP
// penalty. Callers behind a rate-based WAF (e.g. clinch) match it with errors.As to
// stop hydrating for the rest of the run rather than parse the shell as a posting.
type ChallengeError struct {
	URL string
}

func (e *ChallengeError) Error() string {
	return fmt.Sprintf("sources: GET %s: WAF challenge", e.URL)
}
