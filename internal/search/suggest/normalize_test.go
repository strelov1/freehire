package suggest

import "testing"

// The cases here are real posting titles taken from the live catalogue, not invented
// shapes. What a title looks like in the wild is the whole problem: the same job is
// written "Product Owner", "Product owner", "PRODUCT OWNER", "Product Owner (m/f/d)"
// and "Product Owner - Data", and a suggestion dictionary that keeps those apart
// offers five rows where one belongs.
func TestTitle(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already plain", "Product Owner", "product owner"},
		{"case is not part of the name", "PRODUCT OWNER", "product owner"},
		{"mixed case", "Java developer", "java developer"},

		// Everything after the first separator qualifies the job rather than naming it.
		// Keeping it would make one suggestion per employer's punctuation habits.
		{"comma qualifier", "Senior Software Engineer, Infrastructure, Infra Spanner", "senior software engineer"},
		{"pipe qualifier", "Backend Developer | Remote", "backend developer"},
		{"bracket qualifier", "Product Owner (m/f/d)", "product owner"},
		{"square bracket qualifier", "Data Engineer [Remote]", "data engineer"},
		{"slash qualifier", "NodeJS/Fullstack", "nodejs"},
		{"dash qualifier", "Product Owner - Data", "product owner"},
		{"em dash qualifier", "QA Engineer — Warsaw", "qa engineer"},
		{"at qualifier", "Staff Engineer at Google", "staff engineer"},

		// Whitespace is noise from whatever produced the feed.
		{"collapses runs of space", "NodeJS  ReactJS  Developer", "nodejs reactjs developer"},
		{"trims", "  Data Engineer  ", "data engineer"},
		{"newline inside a title", "Product\nOwner", "product owner"},

		// A hyphen INSIDE a word is part of the name — "e-commerce", "front-end" — so
		// only a spaced dash separates. Getting this wrong turns "Front-End Developer"
		// into "front".
		{"hyphen inside a word is kept", "Front-End Developer", "front-end developer"},
		{"hyphenated company-ish word", "E-Commerce Manager", "e-commerce manager"},

		// Characters a name legitimately carries. Dropping them collapses distinct
		// technologies: "C#" and "C" are not the same job.
		{"sharp is part of the name", "C# Developer", "c# developer"},
		{"plus is part of the name", "C++ Engineer", "c++ engineer"},
		{"dot is part of the name", "Node.js Developer", "node.js developer"},

		// A leading dash separates from nothing — it is stray punctuation in front of a
		// real name, so the name survives it.
		{"leading dash is noise, not a separator", "- Data Engineer", "data engineer"},
		// A leading structural separator DOES leave nothing before it, and nothing is
		// what there is to suggest.
		{"leading pipe", "| Remote", ""},
		{"empty", "", ""},
		{"only punctuation", "---", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Title(c.in); got != c.want {
				t.Errorf("Title(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// A typed query and a mined title must land on the same key, or the frequency a
// visitor generates never reaches the suggestion they were reaching for.
func TestTitle_TypedQueryAndMinedTitleAgree(t *testing.T) {
	if Title("Product Owner") != Title("  product owner  ") {
		t.Error("a typed query and the title it names must normalise alike")
	}
}
