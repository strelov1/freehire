package normalize

import (
	"strings"
	"testing"
)

func TestJobSlugWithFullHashSeparatesRealShortcodeCollision(t *testing.T) {
	const firstID, secondID = "collision-224028", "collision-1908612"
	if JobSlug("Backend Engineer", "Acme", "lever", firstID) != JobSlug("Backend Engineer", "Acme", "lever", secondID) {
		t.Fatal("fixture must have a real short-hash collision")
	}
	a := JobSlugWithFullHash("Backend Engineer", "Acme", "lever", firstID)
	b := JobSlugWithFullHash("Backend Engineer", "Acme", "lever", secondID)
	if a == b {
		t.Fatal("full identity hashes must distinguish the colliding postings")
	}
	for _, slug := range []string{a, b} {
		if code, ok := JobSlugCode(slug); !ok || code != "75jwd7n2" {
			t.Fatalf("JobSlugCode(%q) = %q, %v; want the original shortcode", slug, code, ok)
		}
	}
}

func TestJobSlug(t *testing.T) {
	cases := []struct {
		name                         string
		title, company, src, exticID string
		want                         string
	}{
		{
			name:  "title company and shortcode",
			title: "Senior Go Developer", company: "Acme", src: "manual", exticID: "42",
			want: "senior-go-developer-acme-t35nijto",
		},
		{
			name:  "empty company drops the segment, no double hyphen",
			title: "Go Dev", company: "", src: "s", exticID: "1",
			want: "go-dev-5vsxmqi5",
		},
		{
			name:  "empty title and company leaves only the shortcode",
			title: "", company: "", src: "s", exticID: "1",
			want: "5vsxmqi5",
		},
		{
			name:  "distinct external_id changes the shortcode",
			title: "Senior Go Developer", company: "Acme", src: "manual", exticID: "43",
			want: "senior-go-developer-acme-gnhygrtm",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := JobSlug(tc.title, tc.company, tc.src, tc.exticID); got != tc.want {
				t.Errorf("JobSlug(%q,%q,%q,%q) = %q, want %q",
					tc.title, tc.company, tc.src, tc.exticID, got, tc.want)
			}
		})
	}
}

// The shortcode must depend on (source, external_id) only, never on volatile
// fields like the title/company text — so the slug stays stable when only the
// title or company wording changes but the dedup key does not. Here the shortcode
// portion is identical because (source, external_id) is identical.
func TestJobSlugShortcodeIgnoresTitleCompany(t *testing.T) {
	a := JobSlug("Senior Go Developer", "Acme", "manual", "42")
	b := JobSlug("Go Guru", "Acme Inc", "manual", "42")
	const code = "t35nijto"
	if got := a[len(a)-len(code):]; got != code {
		t.Errorf("shortcode of a = %q, want %q", got, code)
	}
	if got := b[len(b)-len(code):]; got != code {
		t.Errorf("shortcode of b = %q, want %q", got, code)
	}
}

// The NUL separator between source and external_id prevents ("ab","c") and
// ("a","bc") from hashing to the same shortcode.
func TestJobSlugSeparatorAvoidsCollision(t *testing.T) {
	if JobSlug("t", "", "ab", "c") == JobSlug("t", "", "a", "bc") {
		t.Error("(ab,c) and (a,bc) must not collide")
	}
}

func TestJobSlugCode(t *testing.T) {
	minted := JobSlug("Senior Go Developer", "Acme", "greenhouse", "123")
	code, ok := JobSlugCode(minted)
	if !ok || code != minted[len(minted)-shortcodeLen:] {
		t.Fatalf("JobSlugCode(%q) = %q, %v; want the minted shortcode", minted, code, ok)
	}
	// The code survives a retitle: it is the one part of the URL that cannot move.
	if other, _ := JobSlugCode(JobSlug("Staff Go Developer", "Acme Inc", "greenhouse", "123")); other != code {
		t.Errorf("code moved with the title: %q vs %q", other, code)
	}
	for _, s := range []string{
		"",
		"senior-go-developer",          // no code segment
		"senior-go-developer-acme-abc", // too short
		"go-developer-acme-abcd1234",   // 1 is not base32
		"go-developer-acme-ABCDEFGH",   // minted codes are lower case
		"go-developer-acmexabcdefgh",   // no separator before the code
	} {
		if c, ok := JobSlugCode(s); ok {
			t.Errorf("JobSlugCode(%q) = %q, true; want no code", s, c)
		}
	}
	if c, ok := JobSlugCode("abcdefgh"); !ok || c != "abcdefgh" {
		t.Errorf("bare code: got %q, %v", c, ok)
	}
}

func TestJobSlugWithFullHashBoundsReadablePrefix(t *testing.T) {
	for _, names := range [][2]string{{strings.Repeat("long-title-", 30), strings.Repeat("long-company-", 30)}, {"", ""}, {"Engineer", "Acme"}} {
		slug := JobSlugWithFullHash(names[0], names[1], "lever", "collision-1908612")
		suffix := JobSlugWithFullHash("", "", "lever", "collision-1908612")
		if len(slug) > 160 || !strings.HasSuffix(slug, suffix) {
			t.Fatalf("fallback must fit return intents and preserve identity: %q", slug)
		}
		if code, ok := JobSlugIdentityCode("old-title-" + suffix); !ok || code != suffix {
			t.Fatalf("extended identity = %q, %v; want %q", code, ok, suffix)
		}
	}
	if code, ok := JobSlugIdentityCode("old-title-75jwd7n2"); !ok || code != "75jwd7n2" {
		t.Fatalf("legacy identity = %q, %v", code, ok)
	}
}
