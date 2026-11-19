package kit

import (
	"testing"
)

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
		// (which SanitizeHTML would then strip, silently losing the content).
		"escaped code sample": {
			"<p>Use <code>&lt;div&gt;&lt;/div&gt;</code> in JSX</p>",
			"<p>Use <code>&lt;div&gt;&lt;/div&gt;</code> in JSX</p>",
		},
		// A bare "<" or "&lt;" used as a less-than sign is not a tag opener, so it never
		// tips the decision either way.
		"less-than in prose": {"<p>salary &lt; 100k</p>", "<p>salary &lt; 100k</p>"},
	}
	for name, c := range cases {
		if got := UnescapeEncodedHTML(c.in); got != c.want {
			t.Errorf("%s: UnescapeEncodedHTML(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}
