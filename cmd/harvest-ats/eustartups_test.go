package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Markup trimmed from live eu-startups.com pages (2026-11).
const eusListingPage = `<style>.wpbdp-listing .listing-title a { color: #044f8d; }</style>
<div class="listing-title">
    <h3><a href="https://www.eu-startups.com/directory/notaryhub/" target="_self" >NotaryHub</a></h3>
</div>
<div class="excerpt-content wpbdp-hide-title"><a href="https://www.eu-startups.com/directory/wpbdp_category/germany-startups/">Germany</a></div>
<div class="listing-title">
    <h3><a href="https://www.eu-startups.com/directory/uptimevanguard/" target="_self" >UptimeVanguard</a></h3>
</div>
<a href="https://www.eu-startups.com/directory/page/3/">Next</a>`

const eusCardPage = `<title>24 Storage &amp; Co | EU-Startups</title>
<div class="wpbdp-field-display wpbdp-field wpbdp-field-value field-display field-value wpbdp-field-founded wpbdp-field-meta wpbdp-field-type-select wpbdp-field-association-meta  " ><span class="field-label">Founded</span> <div class="value">2015</div></div>            <div class="wpbdp-field-display wpbdp-field wpbdp-field-value field-display field-value wpbdp-field-website wpbdp-field-meta wpbdp-field-type-textfield wpbdp-field-association-meta  " ><span class="field-label">Website</span> <div class="value">%s</div></div>`

func TestParseEUSListing(t *testing.T) {
	got := parseEUSListing(eusListingPage)
	want := []string{"notaryhub", "uptimevanguard"} // not the category or pagination links
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := parseEUSListing(`<div class="wpbdp-listings"></div>`); len(got) != 0 {
		t.Errorf("page past the end: got %q, want none", got)
	}
}

func TestParseEUSCard(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"https://24storage.se/", "https://24storage.se/"},
		{"www.24storage.se", "https://www.24storage.se"}, // free-text field: a bare host gets a scheme
		{"", ""},
	} {
		got := parseEUSCard(strings.Replace(eusCardPage, "%s", tc.value, 1))
		if want := (companySite{Name: "24 Storage & Co", Website: tc.want}); got != want {
			t.Errorf("website %q: got %+v, want %+v", tc.value, got, want)
		}
	}
}

func TestLoadEUSStateSkipsTornLine(t *testing.T) {
	st, err := loadEUSState(strings.NewReader(`{"page":1,"slugs":["a","b"]}
{"card":"a","name":"A","website":"https://a.example"}
{"card":"b","name":"B"}
{"card":"c","na`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(st.pages, map[int][]string{1: {"a", "b"}}) {
		t.Errorf("pages = %v", st.pages)
	}
	want := map[string]companySite{"a": {Name: "A", Website: "https://a.example"}, "b": {Name: "B"}}
	if !reflect.DeepEqual(st.cards, want) {
		t.Errorf("cards = %v", st.cards)
	}
}

func TestOpenEUSStateTerminatesTornLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.jsonl")
	if err := os.WriteFile(path, []byte(`{"card":"a","name":"A"}`+"\n"+`{"card":"b","na`), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := openEUSState(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(eusRecord{Card: "c", Name: "C"}); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	r, err := os.Open(path) //nolint:gosec // the test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	st, err := loadEUSState(r)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]companySite{"a": {Name: "A"}, "c": {Name: "C"}}; !reflect.DeepEqual(st.cards, want) {
		t.Errorf("cards = %v, want %v", st.cards, want)
	}
}

func TestWalkEUSResumes(t *testing.T) {
	listing := func(slugs ...string) string {
		var b strings.Builder
		for _, s := range slugs {
			b.WriteString(`<div class="listing-title"><h3><a href="https://www.eu-startups.com/directory/` + s + `/">x</a></h3>`)
		}
		return b.String()
	}
	card := func(name, site string) string {
		return strings.Replace(strings.Replace(eusCardPage, "24 Storage &amp; Co", name, 1), "%s", site, 1)
	}
	pages := map[string]string{
		eusBase + "page/1/": listing("acme", "beta"),
		eusBase + "page/2/": listing("gamma", "acme"), // a card listed twice is fetched once
		eusBase + "page/3/": listing(),
		eusBase + "acme/":   card("Acme", "https://acme.example"), // already in the catalogue
		eusBase + "beta/":   card("Beta", ""),                     // no website
	}
	var (
		mu      sync.Mutex // walkEUS fetches from several workers
		fetched []string
	)
	get := func(u string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		fetched = append(fetched, u)
		if body, ok := pages[u]; ok {
			return body, nil
		}
		return "", errors.New("status 503")
	}
	existing := map[string]bool{"acme": true}
	state := filepath.Join(t.TempDir(), "state.jsonl")

	var out bytes.Buffer
	if code := walkEUS(existing, state, get, &out); code != 0 {
		t.Fatalf("first run exit %d", code)
	}
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Errorf("first run: got %s, want [] (gamma failed)", got)
	}

	pages[eusBase+"gamma/"] = card("Gamma", "gamma.example")
	fetched = nil
	out.Reset()
	if code := walkEUS(existing, state, get, &out); code != 0 {
		t.Fatalf("second run exit %d", code)
	}
	if want := []string{eusBase + "page/3/", eusBase + "gamma/"}; !reflect.DeepEqual(fetched, want) {
		t.Errorf("second run fetched %q, want %q", fetched, want)
	}
	var got []companySite
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if want := []companySite{{Name: "Gamma", Website: "https://gamma.example"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("second run: got %+v, want %+v", got, want)
	}
}

func TestEUSFetcher(t *testing.T) {
	t.Setenv("SCRAPLING_FETCH_URL", "")
	if _, err := eusFetcher(); err == nil {
		t.Error("no SCRAPLING_FETCH_URL: want an error")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ URL string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		status := http.StatusOK
		if strings.HasSuffix(req.URL, "/gone/") {
			status = http.StatusNotFound
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "url": req.URL, "body": base64.StdEncoding.EncodeToString([]byte("page " + req.URL))})
	}))
	defer srv.Close()
	t.Setenv("SCRAPLING_FETCH_URL", srv.URL)
	t.Setenv("SCRAPLING_FETCH_TOKEN", "t")
	get, err := eusFetcher()
	if err != nil {
		t.Fatal(err)
	}
	if body, err := get(eusBase + "acme/"); err != nil || body != "page "+eusBase+"acme/" {
		t.Errorf("200: got %q, %v", body, err)
	}
	if _, err := get(eusBase + "gone/"); err == nil {
		t.Error("404: want an error")
	}
}
