package main

import (
	"context"
	"errors"
	"net/http"

	"testing"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/sources"
)

// erroringClient answers every verb with a canned error, standing in for a platform that
// refuses or a host that never answers.
type erroringClient struct{ err error }

func (c erroringClient) GetJSON(context.Context, string, any) error       { return c.err }
func (c erroringClient) PostJSON(context.Context, string, any, any) error { return c.err }
func (c erroringClient) GetXML(context.Context, string, any) error        { return c.err }
func (c erroringClient) GetHTML(context.Context, string) (*html.Node, error) {
	return nil, c.err
}
func (c erroringClient) GetText(context.Context, string) (string, error) { return "", c.err }
func (c erroringClient) GetJSONWithHeaders(context.Context, string, map[string]string, any) error {
	return c.err
}
func (c erroringClient) PostJSONWithHeaders(context.Context, string, map[string]string, any, any) error {
	return c.err
}

// A prober turns every transport failure into "no such board" — right for a dead host,
// wrong for a refusal. The counting client is the only place that still sees the
// difference, so it has to separate them.
func TestCountingClientSeparatesRefusalsFromOtherFailures(t *testing.T) {
	refused := newCountingClient(erroringClient{
		err: &sources.StatusError{Method: http.MethodGet, Code: http.StatusTooManyRequests, URL: "u"},
	})
	_ = refused.GetJSON(context.Background(), "u", nil)
	if got := refused.refused(); got != 1 {
		t.Errorf("refused = %d, want 1", got)
	}
	if got := refused.answered(); got != 0 {
		t.Errorf("answered = %d, want 0", got)
	}

	dead := newCountingClient(erroringClient{err: errors.New("dial tcp: i/o timeout")})
	_ = dead.GetJSON(context.Background(), "u", nil)
	if got := dead.refused(); got != 0 {
		t.Errorf("a timeout is not a refusal, refused = %d, want 0", got)
	}
}

// A 404 is the platform answering "no such board" — the outcome a harvest is built to read,
// and not a sign anything is wrong with the run.
func TestCountingClientDoesNotCountAbsenceAsRefusal(t *testing.T) {
	c := newCountingClient(erroringClient{
		err: &sources.StatusError{Method: http.MethodGet, Code: http.StatusNotFound, URL: "u"},
	})
	_ = c.GetJSON(context.Background(), "u", nil)
	if got := c.refused(); got != 0 {
		t.Errorf("a 404 is an answer, refused = %d, want 0", got)
	}
}

func TestCountingClientCountsSuccessfulAnswers(t *testing.T) {
	c := newCountingClient(erroringClient{err: nil})
	_ = c.GetJSON(context.Background(), "u", nil)
	_ = c.GetXML(context.Background(), "u", nil)
	if got := c.answered(); got != 2 {
		t.Errorf("answered = %d, want 2", got)
	}
}

// The whole point: a run the platform mostly refused is not a harvest that found nothing,
// and it must not be reported as one.
func TestRefusalsDominated(t *testing.T) {
	if !refusalsDominated(10, 3) {
		t.Error("more refusals than answers must be reported as a refused run")
	}
	if refusalsDominated(3, 10) {
		t.Error("a few refusals among many answers is normal")
	}
	if refusalsDominated(0, 0) {
		t.Error("a run that made no requests is not a refused run")
	}
}

// A 503 is not a rate limit. Traffit answers an unknown tenant with one — 200 for a live
// board, 503 for a slug nobody owns — so counting it as a refusal turned a completely normal
// run (every candidate simply absent) into "refused=15467, nothing written, exit 1".
func TestCountingClientDoesNotCountServiceUnavailableAsRefusal(t *testing.T) {
	c := newCountingClient(erroringClient{
		err: &sources.StatusError{Method: http.MethodGet, Code: http.StatusServiceUnavailable, URL: "u"},
	})
	_ = c.GetJSON(context.Background(), "u", nil)
	if got := c.refused(); got != 0 {
		t.Errorf("a 503 is ambiguous and must not count as a refusal, refused = %d, want 0", got)
	}
	if got := c.answered(); got != 1 {
		t.Errorf("answered = %d, want 1 — the platform did answer", got)
	}
}
