package socialdigest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeTokens struct {
	token string
	err   error
}

func (f fakeTokens) AccessToken(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.token, nil
}

func testDigest(items ...Posting) Digest {
	return Digest{Day: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), Items: items}
}

// linkedInPublisherAt builds a publisher pointed at a stub server.
func linkedInPublisherAt(t *testing.T, url string, tokens TokenSource) *LinkedInPublisher {
	t.Helper()
	p := NewLinkedInPublisher(tokens, "urn:li:organization:130854077", "https://freehire.me")
	p.postsURL = url
	return p
}

func commentaryOf(t *testing.T, p *LinkedInPublisher, d Digest) string {
	t.Helper()
	rendered, err := p.Render(d)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var body struct {
		Commentary string `json:"commentary"`
	}
	if err := json.Unmarshal([]byte(rendered), &body); err != nil {
		t.Fatalf("rendered body is not JSON: %v", err)
	}
	return body.Commentary
}

// Every reserved character has to be escaped even as plain text, and this catalogue's titles
// contain them constantly. An unescaped one is not a cosmetic problem — LinkedIn refuses the
// whole post with a 400, so the day is lost.
func TestLinkedInEscapesEveryReservedCharacter(t *testing.T) {
	p := linkedInPublisherAt(t, "http://127.0.0.1:1/posts", fakeTokens{token: "t"})
	title := `C++ (Senior) [urgent] #1 <now> {x} @team *bold* _under_ ~strike~ a|b back\slash`
	got := commentaryOf(t, p, testDigest(Posting{Slug: "s", Title: title, Company: "Acme"}))

	for _, ch := range []string{`(`, `)`, `[`, `]`, `#`, `<`, `>`, `{`, `}`, `@`, `*`, `_`, `~`, `|`} {
		// Each reserved character must appear preceded by a backslash, and never bare.
		if strings.Contains(got, ch) && !strings.Contains(got, `\`+ch) {
			t.Errorf("reserved %q appears unescaped in %q", ch, got)
		}
	}
	// The backslash must be escaped first, or the escapes written before it get a second
	// backslash and publish as literal characters.
	if !strings.Contains(got, `back\\slash`) {
		t.Errorf("a literal backslash was not doubled: %q", got)
	}
}

// The URL is the one thing that must NOT be escaped: a backslash inside a link is published
// verbatim and the link stops working.
func TestLinkedInLeavesTheJobURLAlone(t *testing.T) {
	p := linkedInPublisherAt(t, "http://127.0.0.1:1/posts", fakeTokens{token: "t"})
	got := commentaryOf(t, p, testDigest(Posting{Slug: "acme-go-1", Title: "Go Engineer", Company: "Acme"}))

	want := "https://freehire.me/jobs/acme-go-1?utm_source=linkedin"
	if !strings.Contains(got, want) {
		t.Errorf("commentary %q does not carry %q", got, want)
	}
	if strings.Contains(got, `\?`) || strings.Contains(got, `\=`) {
		t.Errorf("the URL was escaped: %q", got)
	}
}

// Over the limit the list is trimmed by whole postings. The failure this prevents is a post
// whose last line is half a URL — a link that resolves to nothing, published under our name.
func TestLinkedInTrimsByWholePostings(t *testing.T) {
	p := linkedInPublisherAt(t, "http://127.0.0.1:1/posts", fakeTokens{token: "t"})

	var items []Posting
	for i := range 40 {
		items = append(items, Posting{
			Slug:    fmt.Sprintf("company-%d-very-long-slug-that-eats-the-budget", i),
			Title:   strings.Repeat("Very Senior Staff Engineer ", 4),
			Company: "Company " + strings.Repeat("X", 30),
		})
	}
	got := commentaryOf(t, p, testDigest(items...))

	if n := len([]rune(got)); n > linkedInCommentaryLimit {
		t.Errorf("commentary is %d runes, over the %d limit", n, linkedInCommentaryLimit)
	}
	// A trimmed list ends on a complete URL, never mid-line.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "https://freehire.me/jobs/") || !strings.HasSuffix(last, "?utm_source=linkedin") {
		t.Errorf("the list does not end on a whole link: %q", last)
	}
}

// The payload's required fields are required by the API, not by taste: a missing one is a 400
// MISSING_FIELD at 06:45 UTC. This pins the shape so a later edit cannot quietly drop one.
func TestLinkedInPayloadCarriesTheRequiredFields(t *testing.T) {
	p := linkedInPublisherAt(t, "http://127.0.0.1:1/posts", fakeTokens{token: "t"})
	rendered, err := p.Render(testDigest(Posting{Slug: "s", Title: "T", Company: "C"}))
	if err != nil {
		t.Fatal(err)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(rendered), &body); err != nil {
		t.Fatal(err)
	}
	if body["author"] != "urn:li:organization:130854077" {
		t.Errorf("author = %v", body["author"])
	}
	if body["visibility"] != "PUBLIC" || body["lifecycleState"] != "PUBLISHED" {
		t.Errorf("visibility/lifecycleState = %v/%v", body["visibility"], body["lifecycleState"])
	}
	dist, ok := body["distribution"].(map[string]any)
	if !ok {
		t.Fatalf("distribution is missing: %v", body["distribution"])
	}
	if dist["feedDistribution"] != "MAIN_FEED" {
		t.Errorf("feedDistribution = %v", dist["feedDistribution"])
	}
	// Present and empty, not absent: the API rejects the object without them.
	for _, key := range []string{"targetEntities", "thirdPartyDistributionChannels"} {
		if _, present := dist[key]; !present {
			t.Errorf("distribution.%s is absent, but the API requires it", key)
		}
	}
}

// Render exists so a dry run shows what would really be sent. If the two ever diverge, the dry
// run stops being evidence — so the bodies are compared byte for byte, modulo indentation.
func TestLinkedInRenderMatchesWhatPublishSends(t *testing.T) {
	var sent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = readAll(t, r)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p := linkedInPublisherAt(t, srv.URL, fakeTokens{token: "t"})
	d := testDigest(Posting{Slug: "s", Title: "C++ (Senior)", Company: "Acme", Remote: true})

	if err := p.Publish(context.Background(), d); err != nil {
		t.Fatalf("publish: %v", err)
	}
	rendered, err := p.Render(d)
	if err != nil {
		t.Fatal(err)
	}

	var fromWire, fromRender any
	if err := json.Unmarshal(sent, &fromWire); err != nil {
		t.Fatalf("wire body is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(rendered), &fromRender); err != nil {
		t.Fatalf("rendered body is not JSON: %v", err)
	}
	wire, _ := json.Marshal(fromWire)
	shown, _ := json.Marshal(fromRender)
	if string(wire) != string(shown) {
		t.Errorf("a dry run would show something else:\n sent: %s\nshown: %s", wire, shown)
	}
}

// Two of this API's three requirements are headers, and omitting either is a 400 that names
// neither. Nothing else in the codebase would notice their absence.
func TestLinkedInSendsTheRequiredHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p := linkedInPublisherAt(t, srv.URL, fakeTokens{token: "the-token"})
	if err := p.Publish(context.Background(), testDigest(Posting{Slug: "s", Title: "T", Company: "C"})); err != nil {
		t.Fatalf("publish: %v", err)
	}

	for header, want := range map[string]string{
		"Authorization":             "Bearer the-token",
		"Linkedin-Version":          linkedInAPIVersion,
		"X-Restli-Protocol-Version": "2.0.0",
		"Content-Type":              "application/json",
	} {
		if got.Get(header) != want {
			t.Errorf("%s = %q, want %q", header, got.Get(header), want)
		}
	}
}

// A refused post must say what LinkedIn said. The status alone cannot tell an over-long
// commentary from a malformed URN, and both are 400.
func TestLinkedInReportsTheAPIsOwnComplaint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		writeBody(t, w, `{"code":"FIELD_LENGTH_TOO_LONG","message":"commentary too long"}`)
	}))
	defer srv.Close()

	p := linkedInPublisherAt(t, srv.URL, fakeTokens{token: "t"})
	err := p.Publish(context.Background(), testDigest(Posting{Slug: "s", Title: "T", Company: "C"}))
	if err == nil {
		t.Fatal("a 400 was reported as success")
	}
	if !strings.Contains(err.Error(), "FIELD_LENGTH_TOO_LONG") {
		t.Errorf("error does not carry the API's complaint: %v", err)
	}
}

// A missing or expired credential must fail BEFORE the request, so the error names the
// credential rather than arriving as an unexplained 401.
func TestLinkedInFailsBeforeSendingWithoutACredential(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sentinel := errors.New("no stored credential")
	p := linkedInPublisherAt(t, srv.URL, fakeTokens{err: sentinel})

	err := p.Publish(context.Background(), testDigest(Posting{Slug: "s", Title: "T", Company: "C"}))
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want it to wrap the credential failure", err)
	}
	if called {
		t.Error("a post was attempted without a credential")
	}
}

func readAll(t *testing.T, r *http.Request) []byte {
	t.Helper()
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return body
}

// writeBody writes a stub response, reporting a failed write rather than discarding it.
func writeBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write stub response: %v", err)
	}
}

// The list is ranked on views, so the number belongs in the post — and it is the
// bot-filtered page count, never the fused one migration 0138 split it from.
func TestLinkedInCarriesTheViewCount(t *testing.T) {
	p := linkedInPublisherAt(t, "http://127.0.0.1:1/posts", fakeTokens{token: "t"})

	got := commentaryOf(t, p, testDigest(Posting{Slug: "s", Title: "T", Company: "C", PageUniques: 12}))
	if !strings.Contains(got, "12 views") {
		t.Errorf("commentary does not carry the view count: %q", got)
	}

	// Singular, because "1 views" in a post under our own name is the kind of detail a
	// reader notices instead of the vacancy.
	got = commentaryOf(t, p, testDigest(Posting{Slug: "s", Title: "T", Company: "C", PageUniques: 1}))
	if !strings.Contains(got, "1 view") || strings.Contains(got, "1 views") {
		t.Errorf("a single view is not written in the singular: %q", got)
	}
}
