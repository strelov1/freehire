package classify

import (
	"slices"
	"testing"
)

// TestParse_GTMEngineering pins the go-to-market engineering family to `sales`,
// where revenue-operations titles already sit. The role builds the outbound data
// pipeline rather than selling, but the split is not title-separable from RevOps,
// and the granularity that matters lives in the role dictionary, not here. The
// block sits before the bare `sales` alias, so the guards pin the neighbours it
// must not disturb.
func TestParse_GTMEngineering(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		{"GTM Engineer", "sales"},
		{"Go-To-Market Engineer", "sales"},
		{"Go To Market Engineer", "sales"},
		{"Senior GTM Engineer", "sales"},

		// precision — the neighbours of the inserted block must not shift
		{"Sales Engineer", "solutions_engineering"},
		{"Sales Manager", "sales"},
		{"Revenue Operations Manager", "sales"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_MarketingDisciplines covers the marketing disciplines the category
// dictionary did not name. Most did not merely resolve to nothing — the generic
// "manager" alias further down the table claimed them for `management`, so this
// block corrects wrong data, not just missing data. Every alias is a phrase: the
// bare discipline nouns ("growth", "content", "performance") also occur in
// technical titles, and the guards at the end pin the ones they must not claim.
func TestParse_MarketingDisciplines(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// were claimed by the generic "manager" alias
		{"Demand Generation Manager", "marketing"},
		{"Paid Media Manager", "marketing"},
		{"Community Manager", "marketing"},
		{"PR Manager", "marketing"},
		{"Answer Engine Optimization Manager", "marketing"},
		// resolved to nothing
		{"Growth Marketer", "marketing"},
		{"Paid Social Specialist", "marketing"},
		{"Paid Search Manager", "marketing"},
		{"Media Buyer", "marketing"},
		{"Link Building Specialist", "marketing"},
		{"Content Creator", "marketing"},
		{"Generative Engine Optimization Specialist", "marketing"},
		{"GEO Specialist", "marketing"},

		// precision — a marketing word inside a technical title must not claim it
		{"Growth Engineer", ""},
		{"Content Platform Engineer", "devops"},
		{"Geo Data Analyst", "data_analytics"},
		{"Community Manager, Developer Relations", "developer_relations"},
		// already-resolving marketing titles must not shift
		{"Growth Marketing Manager", "marketing"},
		{"Product Marketing Manager", "marketing"},
		{"Content Designer", "technical_writing"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_MarketingDisciplinesRU covers the Russian marketing titles. As
// elsewhere in this dictionary they are listed as full surface forms rather than
// stems, because the matcher requires word boundaries. Most were claimed by the
// bare "менеджер" alias before this block existed.
func TestParse_MarketingDisciplinesRU(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		{"Таргетолог", "marketing"},
		{"Контент-менеджер", "marketing"},
		{"Бренд-менеджер", "marketing"},
		{"Пиар-менеджер", "marketing"},
		{"Копирайтер", "marketing"},
		{"Комьюнити-менеджер", "marketing"},
		{"SMM-менеджер", "marketing"},

		// already resolving — must not shift
		{"Интернет-маркетолог", "marketing"},
		{"Менеджер по персоналу", "hr"},
		{"Менеджер по продажам", "sales"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// Full-phrase title aliases exist so the web role picker and the header's role
// suggestions can match what people actually type ("backend developer", not just
// "backend") — CategoryAliases feeds the generated web contracts. They must not move
// any title to a different category: each is subsumed by a shorter alias that already
// resolved it, so the classification these tests pin is the classification before them.
func TestSearchPhraseAliasesDoNotChangeClassification(t *testing.T) {
	for _, tc := range []struct{ title, want string }{
		{"Backend Developer", "backend"},
		{"Back-End Developer", "backend"},
		{"Senior Backend Developer", "backend"},
		{"Frontend Developer", "frontend"},
		{"Front-End Developer", "frontend"},
		{"Machine Learning Engineer", "ml_ai"},
		{"Senior Machine Learning Engineer", "ml_ai"},
	} {
		if got := Parse(tc.title).Category; got != tc.want {
			t.Errorf("Parse(%q).Category = %q, want %q", tc.title, got, tc.want)
		}
	}
}

// The same phrases must be REACHABLE as aliases, which is what the search side reads.
func TestCategoryAliasesCarrySearchPhrases(t *testing.T) {
	aliases := CategoryAliases()
	for _, tc := range []struct{ canonical, alias string }{
		{"backend", "backend developer"},
		{"frontend", "frontend developer"},
		{"ml_ai", "machine learning engineer"},
	} {
		if !slices.Contains(aliases[tc.canonical], tc.alias) {
			t.Errorf("CategoryAliases()[%q] missing %q", tc.canonical, tc.alias)
		}
	}
}

// TestParse_CreativeMedia pins the media-production vocabulary: the video, art,
// animation, audio and photography titles that resolved to NO category before the
// `creative` category existed, and the boundary that keeps it from taking rows away
// from `design` and `marketing`.
func TestParse_CreativeMedia(t *testing.T) {
	cases := []struct {
		title        string
		wantCategory string
		why          string
	}{
		// The video family.
		{"Video Editor", "creative", "the craft this catalogue could not name"},
		{"Senior Video Editor", "creative", "grade does not change the craft"},
		{"Video Producer", "creative", ""},
		{"Videographer", "creative", ""},

		// The art family. Each is a distinct seat on a games or media team.
		{"3D Artist", "creative", ""},
		{"2D Artist", "creative", ""},
		{"Concept Artist", "creative", ""},
		{"Character Artist", "creative", ""},
		{"Environment Artist", "creative", ""},
		{"Technical Artist", "creative", ""},
		{"VFX Artist", "creative", ""},
		{"Storyboard Artist", "creative", ""},

		// Animation.
		{"Animator", "creative", ""},
		{"3D Animator", "creative", ""},
		{"2D Animator", "creative", ""},
		{"Motion Graphics Artist", "creative", "the artist spelling; the designer spelling stays in design"},

		// Audio: the one population that MOVES, out of design. All three spellings go
		// together — leaving any behind scatters one craft across three categories.
		{"Sound Designer", "creative", "was design, for no reason but the word designer"},
		{"Audio Designer", "creative", ""},
		{"Sound Design Engineer", "creative", "was design"},
		{"Audio Design Engineer", "creative", "fell through to engineering_design"},
		// NOT audio: "Audio Engineer" and "Sound Engineer" are broadcast, live sound and
		// AV integration as often as they are the craft, so neither is an alias.
		{"Audio Engineer", "", ""},
		// Still not `creative`, which is what this case guards. It now resolves to
		// `industrial_engineering` instead of nothing: the title names a field service
		// seat, and that category did not exist when this case was written.
		{"Field Service Engineer - Audio Engineer", "industrial_engineering", ""},

		// Photography.
		{"Photographer", "creative", ""},
		{"Photo Editor", "creative", ""},
		{"Illustrator", "creative", "the job title, not the Adobe product"},

		// The boundary with design: these keep the category they have today.
		{"Motion Designer", "design", ""},
		{"Motion Graphics Designer", "design", ""},
		{"Graphic Designer", "design", ""},
		{"Visual Designer", "design", ""},
		{"Brand Designer", "design", ""},
		{"Product Designer", "design", ""},
		{"Senior UX Designer", "design", ""},
		// Game development keeps the categories it resolves to today — there is no game
		// category, so these are the rows the named roles have to hang off.
		{"Game Designer", "design", "a game category would take rows from two working facets"},
		{"Narrative Designer", "design", ""},
		{"Level Designer", "design", ""},
		{"Game Developer", "software_engineering", ""},
		{"Game Producer", "", "no category resolves; the named role carries it alone"},
		// The craft aliases are declared LAST, so a title naming any other discipline
		// keeps it. Each of these is a tool or a second hat inside someone else's title.
		{"Graphic Designer (Illustrator, Photoshop)", "design", ""},
		{"Graphic Designer & Photographer", "design", ""},
		{"Junior Motion Designer / Animator", "design", ""},

		// The boundary with marketing — same rule, the other side of the table.
		{"Content Creator", "marketing", ""},
		{"UGC Creator", "", "resolves no category today and must not start resolving to creative"},
		{"Video Marketing Manager", "marketing", "marketing that uses video is still marketing"},
		{"Marketing Specialist (Photoshop, Illustrator)", "marketing", ""},
		{"Social Media Manager (Canva, Illustrator)", "marketing", ""},
		{"Content Creator / Illustrator", "marketing", ""},
		{"Social Media Video Editor", "marketing",
			"the stated cost of declaring the crafts last: still findable, on a facet already correct for it"},

		// The boundary with engineering: a bare craft word never resolves on its own.
		{"Audio DSP Engineer", "", "bare \"audio\" is not an alias"},
		{"Mechanical Design Engineer", "engineering_design", ""},
		{"Art Director", "", "resolves no category today; the bare word \"art\" must not start one"},
	}

	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}
