package main

import (
	"reflect"
	"strings"
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
