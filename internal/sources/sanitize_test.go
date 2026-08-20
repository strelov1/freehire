package sources

import (
	"strings"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	in := `<h2>Role</h2><p>Lead the <strong>backend</strong> team.</p>` +
		`<ul><li>Ship features</li></ul>` +
		`<a href="https://example.com" onclick="steal()">apply</a>` +
		`<img src="https://evil.example/track.gif">` +
		`<script>alert(1)</script>`

	got := sanitizeHTML(in)

	// Structural formatting is preserved.
	for _, want := range []string{"<h2>Role</h2>", "<strong>backend</strong>", "<li>Ship features</li>", "apply"} {
		if !strings.Contains(got, want) {
			t.Errorf("sanitizeHTML dropped expected markup %q\ngot: %s", want, got)
		}
	}

	// Active content and external request vectors are stripped.
	for _, bad := range []string{"<script", "onclick", "alert(1)", "<img", "track.gif"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitizeHTML kept unsafe content %q\ngot: %s", bad, got)
		}
	}

	// Links carry no destination at all: the anchor is unwrapped to its text.
	for _, bad := range []string{"<a ", "href=", "example.com", "nofollow"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitizeHTML kept link markup %q\ngot: %s", bad, got)
		}
	}
}

// Descriptions are prose, not a link farm: every anchor is unwrapped to its text, so a
// posting can neither route a reader off the catalogue nor pass link authority. What the
// anchor SAID still matters, though — aggregators weave backlinks through ordinary
// sentences ("we use <a>Kubernetes</a>"), and dropping the text with the tag would punch
// holes in the prose. The one exception is an anchor whose text is itself a bare URL:
// stripped of its href that is not prose at all, just an unclickable address, so it goes
// with the tag.
func TestSanitizeHTMLStripsLinks(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"anchor unwrapped to its text": {
			`<p>We use <a href="https://k8s.io">Kubernetes</a> in prod.</p>`,
			`<p>We use Kubernetes in prod.</p>`,
		},
		"nested markup survives the unwrap": {
			`<p>See <a href="https://x.co"><strong>the docs</strong></a>.</p>`,
			`<p>See <strong>the docs</strong>.</p>`,
		},
		// The unwrap deletes the tags rather than replacing them with a space, so a linked
		// word keeps its punctuation glued exactly as the employer wrote it. Any drift here
		// would change visible text and so change role_fingerprint (see internal/jobhash).
		"unwrap keeps punctuation glued": {
			`<p>an agreement with <a href="https://x.co">Oasis</a>.</p>`,
			`<p>an agreement with Oasis.</p>`,
		},
		"bare URL text goes with the tag": {
			`<p>Apply here: <a href="https://boards.x.co/1">https://boards.x.co/1</a></p>`,
			`<p>Apply here: </p>`,
		},
		"schemeless www text goes too": {
			`<p>More: <a href="https://www.x.co">www.x.co/jobs</a></p>`,
			`<p>More: </p>`,
		},
		"bare domain text goes too": {
			`<p>Visit <a href="https://arbeitnow.com">arbeitnow.com</a></p>`,
			`<p>Visit </p>`,
		},
		// A contact address is prose the reader needs, even though it looks address-like.
		"mailto text is kept": {
			`<p>Send your CV to <a href="mailto:hr@x.co">hr@x.co</a></p>`,
			`<p>Send your CV to hr@x.co</p>`,
		},
		// The aggregator footer the catalogue is full of: the words stay, the link does not.
		"promo trailer keeps its words": {
			`<p><a href="https://www.arbeitnow.com/">Find Jobs in Germany on Arbeitnow</a></p>`,
			`<p>Find Jobs in Germany on Arbeitnow</p>`,
		},
		// A block left empty by a dropped bare-URL anchor would render as a stray gap.
		"block emptied by the drop is removed": {
			`<p>Role</p><p><a href="https://x.co/1">https://x.co/1</a></p>`,
			`<p>Role</p>`,
		},
		"list item emptied by the drop is removed": {
			`<ul><li>Go</li><li><a href="https://x.co/1">https://x.co/1</a></li></ul>`,
			`<ul><li>Go</li></ul>`,
		},
		// Nothing to strip: a body without anchors must survive byte for byte.
		"link-free body is untouched": {
			`<p>Build <strong>things</strong>.</p>`,
			`<p>Build <strong>things</strong>.</p>`,
		},
	}
	for name, c := range cases {
		if got := sanitizeHTML(c.in); got != c.want {
			t.Errorf("%s: sanitizeHTML(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

// The backfill re-runs this pipeline over rows the ingest path already sanitized, so a
// second pass must be a no-op — otherwise every run would rewrite the whole catalogue.
func TestSanitizeHTMLLinkStripIsIdempotent(t *testing.T) {
	in := `<p>Apply: <a href="https://x.co/1">https://x.co/1</a> or ask <a href="mailto:hr@x.co">hr@x.co</a></p>`
	once := sanitizeHTML(in)
	if twice := sanitizeHTML(once); twice != once {
		t.Errorf("sanitizeHTML is not idempotent:\nonce:  %q\ntwice: %q", once, twice)
	}
}

// Some ATS boards emit descriptions whose words are glued by non-breaking spaces
// (U+00A0, often as the &nbsp; entity) instead of regular spaces. Rendered with
// {@html} this becomes one unbreakable line that overflows the page horizontally,
// so the sanitizer normalizes no-break spaces to regular ones, restoring word-boundary
// wrapping. bluemonday decodes &nbsp; to the raw U+00A0 rune, so normalizing the
// sanitized output catches both the entity and raw-character forms.
func TestSanitizeHTMLNormalizesNoBreakSpaces(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"entity form":   {"<p>Java&nbsp;Spring&nbsp;Boot</p>", "<p>Java Spring Boot</p>"},
		"raw U+00A0":    {"<p>Java Spring Boot</p>", "<p>Java Spring Boot</p>"},
		"narrow U+202F": {"<p>5 years</p>", "<p>5 years</p>"},
	}
	for name, c := range cases {
		if got := sanitizeHTML(c.in); got != c.want {
			t.Errorf("%s: sanitizeHTML(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

// Some ATS boards emit a description whose list wrapper drops out partway through —
// seen live on a real posting: two properly-wrapped bullet groups followed by a third
// group of bare <li> siblings with no <ul>/<ol> around them at all. bluemonday allows
// <li> as a structural element but does not require a list parent, so the malformed
// shape survived sanitization unchanged and Lighthouse flags it (a <li> outside a list
// has no list semantics for a screen reader). sanitizeHTML now wraps any such orphan
// run in a synthetic <ul>, and leaves an already-correct <ul> byte-for-byte untouched.
func TestSanitizeHTMLWrapsOrphanListItems(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"bare li with no list at all": {
			`<p><strong>Qualifications:</strong></p><li>A</li><li>B</li>`,
			`<p><strong>Qualifications:</strong></p><ul><li>A</li><li>B</li></ul>`,
		},
		"already-wrapped list is untouched": {
			`<ul><li>Ship features</li></ul>`,
			`<ul><li>Ship features</li></ul>`,
		},
		"mixed real-world shape: two valid lists then an orphan run": {
			"<p><strong>Minimum Qualifications:</strong></p>\n<ul>\n <li>Nursing degree</li>\n <li>RN license</li>\n</ul>\n" +
				"<p><strong>Essential Functions:</strong></p>\n<li>Acts as liaison</li>\n<li>Coordinates care</li>",
			"<p><strong>Minimum Qualifications:</strong></p>\n<ul>\n <li>Nursing degree</li>\n <li>RN license</li>\n</ul>\n" +
				"<p><strong>Essential Functions:</strong></p><ul>\n<li>Acts as liaison</li>\n<li>Coordinates care</li></ul>",
		},
	}
	for name, c := range cases {
		if got := sanitizeHTML(c.in); got != c.want {
			t.Errorf("%s: sanitizeHTML(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

// The backfill re-runs this pipeline over already-sanitized rows, so wrapping must be
// a no-op on its own output — otherwise every run would keep rewriting the catalogue.
func TestSanitizeHTMLWrapsOrphanListItemsIsIdempotent(t *testing.T) {
	in := `<p>Qualifications:</p><li>A</li><li>B</li>`
	once := sanitizeHTML(in)
	if twice := sanitizeHTML(once); twice != once {
		t.Errorf("sanitizeHTML is not idempotent:\nonce:  %q\ntwice: %q", once, twice)
	}
}

// Some aggregator feeds double-escape the newlines in their JSON, so the decoded description
// carries the two characters "\" and "n" where the source HTML had a line break — visible to
// the reader as literal "\n" in the middle of the prose (seen live on whatjobs and adzuna
// postings resold through the same upstream network). What the escape MEANT depends on where
// it sits, which is why the repair is not a single replacement: next to a tag it is the source
// file's indentation and collapses like any other whitespace, while between two sentences it
// is the only line break the posting has and must survive as a <br>.
func TestSanitizeHTMLRepairsLiteralNewlines(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"indentation between blocks collapses": {
			`<p>Company Description</p>\n<p>We are hiring.</p>`,
			"<p>Company Description</p>\n<p>We are hiring.</p>",
		},
		"indentation before a list item collapses": {
			`<ul>\n <li>Go</li>\n <li>SQL</li>\n</ul>`,
			"<ul>\n <li>Go</li>\n <li>SQL</li>\n</ul>",
		},
		"escape between bare text and a tag collapses": {
			`<p>Was du bewegst\n<p>Deine Aufgaben</p>`,
			"<p>Was du bewegst\n<p>Deine Aufgaben</p>",
		},
		"escape between two sentences becomes a break": {
			`<p>~ Kita-Zuschuss \n~ Weihnachtsgeld</p>`,
			`<p>~ Kita-Zuschuss <br>~ Weihnachtsgeld</p>`,
		},
		"body with no escape is untouched": {
			`<p>Ship features</p>`,
			`<p>Ship features</p>`,
		},
	}
	for name, c := range cases {
		if got := sanitizeHTML(c.in); got != c.want {
			t.Errorf("%s: sanitizeHTML(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

// The backfill re-runs this pipeline over already-repaired rows, so a body that carries no
// escape any more must come back byte-for-byte — otherwise every run would rewrite the same
// rows and each rewrite is a TOAST write.
func TestSanitizeHTMLRepairsLiteralNewlinesIsIdempotent(t *testing.T) {
	in := `<p>~ Kita-Zuschuss \n~ Weihnachtsgeld</p>\n<p>Bewirb dich.</p>`
	once := sanitizeHTML(in)
	if twice := sanitizeHTML(once); twice != once {
		t.Errorf("sanitizeHTML is not idempotent:\nonce:  %q\ntwice: %q", once, twice)
	}
}

func TestLenientPercentUnescape(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"plain":              {"hello world", "hello world"},
		"valid escapes":      {"%3Cp%3Ehi%3C%2Fp%3E", "<p>hi</p>"},
		"literal percent":    {"line-height:115%;color", "line-height:115%;color"},
		"stat percent":       {"100% remote", "100% remote"},
		"mixed":              {"%3Cb%3E100%25 %3D%3E all%3C%2Fb%3E", "<b>100% => all</b>"},
		"plus preserved":     {"C%2B%2B and C++", "C++ and C++"},
		"trailing lone pct":  {"done 50%", "done 50%"},
		"lone pct then hex1": {"%3 only", "%3 only"},
		"utf8 bytes":         {"%D0%9F%D1%80%D0%B8%D0%B2%D0%B5%D1%82", "Привет"},
	}
	for name, c := range cases {
		if got := LenientPercentUnescape(c.in); got != c.want {
			t.Errorf("%s: LenientPercentUnescape(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

func TestUnescapeEncodedHTML(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		// Live markup is the common case and must survive byte for byte: unescaping it
		// would decode entities the posting meant literally.
		"live markup":        {"<p>Build &amp; ship.</p>", "<p>Build &amp; ship.</p>"},
		"entities, no tags":  {"R&amp;D team", "R&amp;D team"},
		"plain text":         {"Just text", "Just text"},
		"encoded body":       {"&lt;p&gt;Hi&lt;/p&gt;", "<p>Hi</p>"},
		"encoded attributes": {"&lt;p class=&quot;x&quot;&gt;Hi&lt;/p&gt;", `<p class="x">Hi</p>`},
		// The shape arbeitnow actually serves: an encoded employer body followed by the
		// board's own live-HTML promo footer. Encoded openers dominate, so the whole
		// string is decoded — a no-op on the footer, which carries no entities.
		"encoded body, live footer": {
			`&lt;p&gt;Role&lt;/p&gt;&lt;ul&gt;&lt;li&gt;Go&lt;/li&gt;&lt;/ul&gt;<p>Find more <a href="x">jobs</a></p>`,
			`<p>Role</p><ul><li>Go</li></ul><p>Find more <a href="x">jobs</a></p>`,
		},
		// A posting that deliberately shows markup as an example: live openers outnumber
		// encoded ones, so the example is left encoded instead of becoming real tags
		// (which sanitizeHTML would then strip, silently losing the content).
		"escaped code sample": {
			"<p>Use <code>&lt;div&gt;&lt;/div&gt;</code> in JSX</p>",
			"<p>Use <code>&lt;div&gt;&lt;/div&gt;</code> in JSX</p>",
		},
		// A bare "<" or "&lt;" used as a less-than sign is not a tag opener, so it never
		// tips the decision either way.
		"less-than in prose": {"<p>salary &lt; 100k</p>", "<p>salary &lt; 100k</p>"},
	}
	for name, c := range cases {
		if got := unescapeEncodedHTML(c.in); got != c.want {
			t.Errorf("%s: unescapeEncodedHTML(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}
