package collections

import (
	"reflect"
	"sort"
	"testing"
)

func TestPortfolioParsers_KeepCurrentAndPublicDropAcquired(t *testing.T) {
	cases := []struct {
		name  string
		parse func([]byte) ([]Record, error)
		data  string
		want  []Record
	}{
		{
			name:  "greylock",
			parse: ParseGreylock,
			data: `{"result":[
				{"name":"Parallax","website":"https://parallax.ai/","status":"Active"},
				{"name":"Discord","website":"https://discord.com","status":"Public"},
				{"name":"Gone","website":"https://gone.com","status":"Acquired"}]}`,
			want: []Record{record("Parallax", "https://parallax.ai/"), record("Discord", "https://discord.com")},
		},
		{
			name:  "usv",
			parse: ParseUSV,
			data: `{"result":[
				{"name":"Abridge","website":"https://www.abridge.com/","currentStage":"growth"},
				{"name":"Old","url":"http://old.com","currentStage":"public"},
				{"name":"Sold","website":"https://sold.com","currentStage":"acquired"},
				{"name":"Dead","website":"https://dead.com","currentStage":"inactive"}]}`,
			want: []Record{record("Abridge", "https://www.abridge.com/"), record("Old", "http://old.com")},
		},
		{
			name:  "accel",
			parse: ParseAccel,
			data: `{"result":[
				{"name":"Viktor","websiteUrl":"https://getviktor.com/","archived":false},
				{"name":"Archived","websiteUrl":"https://a.com","archived":true}]}`,
			want: []Record{record("Viktor", "https://getviktor.com/")},
		},
		{
			name:  "bond",
			parse: ParseBond,
			data: `{"data":{
				"investments_bond":{"edges":[
					{"node":{"name":"AKASA","url":"//akasa.com/"}},
					{"node":{"name":"CLEAR (NYSE: YOU)","url":"//clearme.com/"}},
					{"node":{"name":"Weights & Biases (CoreWeave acquired)","url":"//wandb.ai/"}}]},
				"investments_kp":{"edges":[{"node":{"name":"Legacy Fund Co","url":"//legacy.com/"}}]}}}`,
			want: []Record{record("AKASA", "//akasa.com/"), record("CLEAR", "//clearme.com/")},
		},
		{
			name:  "a16z",
			parse: ParseA16zPortfolio,
			data: `<div data-companies="[{&quot;name&quot;:&quot;SpaceX&quot;,&quot;external_url&quot;:&quot;https://spacex.com&quot;,&quot;stages&quot;:[&quot;IPO&quot;]},` +
				`{&quot;name&quot;:&quot;Eka Robotics&quot;,&quot;external_url&quot;:&quot;https://ekarobotics.com&quot;,&quot;stages&quot;:&quot;&quot;},` +
				`{&quot;name&quot;:&quot;Bought&quot;,&quot;external_url&quot;:&quot;https://bought.com&quot;,&quot;stages&quot;:[&quot;M&amp;amp;A&quot;]}]"></div>`,
			want: []Record{record("SpaceX", "https://spacex.com"), record("Eka Robotics", "https://ekarobotics.com")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.parse([]byte(tc.data))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("records = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestParseSequoia_ReadsCompanyPagesOnly(t *testing.T) {
	data := []byte(`{
		"/companies/nxyz": {"title": "nxyz | Sequoia Capital", "p": ["our companies", "nxyz indexes the blockchain.", "n.xyz", "Founded 2022", "Acquired 2023"]},
		"/companies/evy": {"title": "Evy | Sequoia Capital", "p": ["our companies", "Evy builds things.", "evy.eu", "Founded 2021", "Partnered 2022"]},
		"/companies/airbnb": {"title": "Airbnb | Sequoia Capital", "p": ["Airbnb hosts.", "airbnb.com", "IPO 2020"]},
		"/our-companies": {"title": "Our Companies | Sequoia Capital", "p": ["x.com"]}
	}`)
	got, err := ParseSequoia(data)
	if err != nil {
		t.Fatalf("ParseSequoia: %v", err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	want := []Record{record("Airbnb", "airbnb.com"), record("Evy", "evy.eu")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("records = %#v, want %#v", got, want)
	}
}

func TestPortfolioParsers_RejectGarbage(t *testing.T) {
	for name, parse := range map[string]func([]byte) ([]Record, error){
		"greylock": ParseGreylock, "usv": ParseUSV, "accel": ParseAccel, "bond": ParseBond,
		"a16z": ParseA16zPortfolio, "sequoia": ParseSequoia,
	} {
		if _, err := parse([]byte("not json")); err == nil {
			t.Errorf("%s accepted garbage", name)
		}
	}
}

func TestSameSite(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://stripe.com", "stripe.com", true},
		{"https://careers.stripe.com/jobs", "http://www.stripe.com", true},
		{"stripe.com", "stripe.co", true},
		{"acme.co.uk", "https://www.acme.com", true},
		{"http://www.metavision.com/", "https://www.meta.com", false},
		{"https://withbenchmark.com/", "https://benchmark.com", false},
		{"", "https://benchmark.com", true},
		{"https://benchmark.com", "", true},
		{"not a host", "https://benchmark.com", true},
		{"https://redditinc.com", "http://reddit.com", true},
		{"https://www.yahoo.com", "https://yahooinc.com", true},
		{"google.com", "https://about.google", true},
		{"sunflower-labs.com", "https://sunflowerlabs.com", true},
		{"https://www.groupon.com/", "job-boards.eu.greenhouse.io", true},
		{"https://mesh.ai/", "http://www.usemesh.com", false},
		{"http://apollographql.com/", "https://www.apollofirst.xyz/", false},
	}
	for _, tc := range cases {
		if got := sameSite(tc.a, tc.b); got != tc.want {
			t.Errorf("sameSite(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMembers_RefusesANameMatchRunningADifferentSite(t *testing.T) {
	c := Collection{Slug: "yc", Kind: KindBacker}
	companies := map[string]Company{
		"meta":   {Slug: "meta", Website: "https://www.meta.com"},
		"stripe": {Slug: "stripe", Website: "http://stripe.com"},
		"acme":   {Slug: "acme"},
	}
	records := []Record{
		record("Meta", "http://www.metavision.com/"),
		record("Stripe", "https://stripe.com"),
		record("Acme", "https://acme.io"),
	}
	got, stat := c.Members(records, companies)
	if want := []string{"acme", "stripe"}; !reflect.DeepEqual(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if stat.Mismatch != 1 {
		t.Errorf("Mismatch = %d, want 1", stat.Mismatch)
	}
}

func TestPlausibleSize(t *testing.T) {
	cases := []struct {
		team     string
		openJobs int
		want     bool
	}{
		{"2", 693, false},   // YC's Pennant against the Pennant Group's nursing postings
		{"18", 534, false},  // YC's Arrow against Arrow Electronics
		{"40", 91, true},    // Finni Health, the busiest real YC member hiring
		{"7000", 558, true}, // Stripe
		{"2", 12, true},     // a small startup under the floor
		{"1", 26, true},     // ShipBob: the directory's headcount is stale, not the company
		{"4", 53, false},
		{"", 900, true}, // no published headcount refuses nothing
		{"n/a", 900, true},
	}
	for _, tc := range cases {
		if got := plausibleSize(tc.team, tc.openJobs); got != tc.want {
			t.Errorf("plausibleSize(%q, %d) = %v, want %v", tc.team, tc.openJobs, got, tc.want)
		}
	}
}

func TestMembers_RefusesANameMatchFarLargerThanTheMember(t *testing.T) {
	c := Collection{Slug: "yc", Kind: KindBacker}
	companies := map[string]Company{
		"pennant": {Slug: "pennant", OpenJobs: 693},
		"stripe":  {Slug: "stripe", OpenJobs: 558},
	}
	pennant := record("Pennant", "https://www.getpennant.ai")
	pennant.Meta[MetaTeamSize] = "2"
	stripe := record("Stripe", "http://stripe.com")
	stripe.Meta[MetaTeamSize] = "7000"
	got, stat := c.Members([]Record{pennant, stripe}, companies)
	if want := []string{"stripe"}; !reflect.DeepEqual(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if stat.Mismatch != 1 || len(stat.MismatchNames) != 1 {
		t.Errorf("Mismatch = %d names=%v, want one refusal", stat.Mismatch, stat.MismatchNames)
	}
}

func TestHTMLPortfolioParsers(t *testing.T) {
	cases := []struct {
		name  string
		parse func([]byte) ([]Record, error)
		data  string
		want  []string
	}{
		{
			name:  "lightspeed",
			parse: ParseLightspeed,
			data: `<div data-company-id="wiz" data-spotlight-founders-list><h6>Co-Founder, Wiz</h6></div>
				<ul><li data-appear-list data-company-id="affirm" data-investor="lsvp"><h5> Affirm </h5>
				<ul class="company-info-list"><li><strong>Status</strong> <span>IPO</span></li></ul></li>
				<li data-appear-list data-company-id="gone" data-investor="lsip"><h5>Gone &amp; Co</h5>
				<ul class="company-info-list"><li><strong>Status</strong> <span>Acquired</span></li></ul></li>
				<li data-appear-list data-company-id="alloy" data-investor="both"><h5>Alloy <span class="info-icon-wrapper"><img src="i.svg"><span class="disclaimer-text"><p>LSVP and LSIP Investment</p></span></span></h5>
				<ul class="company-info-list"><li><strong>Status</strong> <span>Private</span></li></ul></li></ul>`,
			want: []string{"Affirm", "Alloy"},
		},
		{
			name:  "index",
			parse: ParseIndex,
			data: `<li class="companies__relationships__list__item js-company"><a href="/companies/1stdibs/" class="companies__relationships__list__item__link"> 1stdibs <span class="ticker-symbol"> NASDAQ: DIBS </span> </a></li>
				<li class="companies__relationships__list__item js-company"><a href="/companies/abacus/" class="companies__relationships__list__item__link"> Abacus.ai </a></li>`,
			want: []string{"1stdibs", "Abacus.ai"},
		},
		{
			name:  "vision fund",
			parse: ParseVisionFund,
			data: `<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"entry":{"modules":[{"title":"hero"},
				{"portfolio_companies":{"data":[{"title":"Guazi","company_status":{"title":"Private"}},
				{"title":"Sold","company_status":{"title":"Acquired"}},{"title":"IonQ","company_status":{"title":"Public"}}]}}]}}}}</script>`,
			want: []string{"Guazi", "IonQ"},
		},
		{
			name:  "founders fund",
			parse: ParseFoundersFund,
			data:  `[{"title":{"rendered":"SpaceX"}},{"title":{"rendered":"Trade Republic &#038; Co"}}]`,
			want:  []string{"SpaceX", "Trade Republic & Co"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.parse([]byte(tc.data))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			names := make([]string, len(got))
			for i, r := range got {
				names[i] = r.Name
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Errorf("names = %v, want %v", names, tc.want)
			}
		})
	}
	for name, parse := range map[string]func([]byte) ([]Record, error){
		"lightspeed": ParseLightspeed, "index": ParseIndex, "vision fund": ParseVisionFund, "founders fund": ParseFoundersFund,
	} {
		if _, err := parse([]byte("<html>nothing here</html>")); err == nil {
			t.Errorf("%s accepted a page with no portfolio", name)
		}
	}
}
