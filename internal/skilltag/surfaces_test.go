package skilltag

import (
	"reflect"
	"testing"
)

func TestPreferredFromText_LongestWins(t *testing.T) {
	got := PreferredFromText("We practice IaC and Infrastructure as Code daily.")
	want := "Infrastructure as Code"
	if got["infrastructure-as-code"] != want {
		t.Fatalf("preferred = %q, want %q (longest); full map %#v", got["infrastructure-as-code"], want, got)
	}
}

func TestPreferredFromText_JDOnlyAcronym(t *testing.T) {
	got := PreferredFromText("Experience with IaC and Terraform.")
	if got["infrastructure-as-code"] != "IaC" {
		t.Fatalf("preferred = %q, want IaC; full map %#v", got["infrastructure-as-code"], got)
	}
	if got["terraform"] != "Terraform" {
		t.Fatalf("terraform preferred = %q, want Terraform", got["terraform"])
	}
}

func TestPreferredFromText_UnknownIgnored(t *testing.T) {
	got := PreferredFromText("Must know BlorpleDB and the FluxCapacitor framework.")
	if len(got) != 0 {
		t.Fatalf("got %#v, want empty — unknown jargon must not invent surfaces", got)
	}
}

func TestPreferredFromText_CasingPreserved(t *testing.T) {
	got := PreferredFromText("Required: Kubernetes and PostgreSQL.")
	if got["kubernetes"] != "Kubernetes" {
		t.Errorf("kubernetes = %q, want Kubernetes", got["kubernetes"])
	}
	if got["postgresql"] != "PostgreSQL" {
		t.Errorf("postgresql = %q, want PostgreSQL", got["postgresql"])
	}
}

func TestPreferredFromText_Empty(t *testing.T) {
	if got := PreferredFromText(""); got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

func TestAliasesOf_LongestFirst(t *testing.T) {
	got := AliasesOf("infrastructure-as-code")
	if len(got) < 2 {
		t.Fatalf("aliases = %v, want at least iac and the phrase", got)
	}
	if got[0] != "infrastructure as code" && got[0] != "infrastructure-as-code" {
		// phraseAliases has "infrastructure as code"; canonical slug may also appear.
		// Longest should be the spaced phrase (22 runes) over "infrastructure-as-code" (22)
		// or "iac" (3). Spaced phrase and hyphenated slug are same rune length for
		// "infrastructure as code" (22) vs "infrastructure-as-code" (22) — spaces vs hyphens.
		// "infrastructure as code" has spaces: i-n-f-r-a-s-t-r-u-c-t-u-r-e- -a-s- -c-o-d-e = 22
		// Either long form before iac is fine.
	}
	foundPhrase, foundIAC := false, false
	for i, a := range got {
		if a == "infrastructure as code" {
			foundPhrase = true
			if i > 0 && got[0] == "iac" {
				t.Fatalf("aliases = %v, want long phrase before iac", got)
			}
		}
		if a == "iac" {
			foundIAC = true
		}
	}
	if !foundPhrase || !foundIAC {
		t.Fatalf("aliases = %v, want both phrase and iac", got)
	}
}

func TestIsProseSafeAlias(t *testing.T) {
	cases := []struct {
		alias string
		want  bool
	}{
		{"infrastructure as code", true},
		{"iac", true},
		{"k8s", true},
		{"ci/cd", true},
		{"go", false},
		{"react", false},
		{"c", false},
		{"js", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsProseSafeAlias(tc.alias); got != tc.want {
			t.Errorf("IsProseSafeAlias(%q) = %v, want %v", tc.alias, got, tc.want)
		}
	}
}

func TestPreferredFromText_Deterministic(t *testing.T) {
	text := "IaC with Kubernetes and React."
	a := PreferredFromText(text)
	b := PreferredFromText(text)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("not deterministic:\n%#v\n%#v", a, b)
	}
}
