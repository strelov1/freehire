package normalize

import "testing"

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
