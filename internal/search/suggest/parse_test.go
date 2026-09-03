package suggest

import (
	"slices"
	"testing"
)

// Built through the constructor rather than by filling the struct: the parse's
// look-ahead bound is derived there, and a hand-built set would silently recognise
// nothing.
func phrases(p ...string) *Phrases {
	docs := make([]Document, 0, len(p))
	for _, s := range p {
		docs = append(docs, Document{Kind: KindCategory, Slug: Title(s), Text: s, Jobs: 1})
	}
	return NewPhrases(docs)
}

// The box completes a PHRASE, not a word. Typing `senior` offers "Senior Software
// Engineer"; continuing to `senior software engineer go` must offer that role plus
// Google rather than starting over. Verified end to end against production:
// `q=senior software engineer` with `company_slug=google` returns 871 postings, so the
// composed filter is real.
func TestParse(t *testing.T) {
	ph := phrases("senior software engineer", "product owner")

	t.Run("nothing recognised yet", func(t *testing.T) {
		got := ph.Parse("seni")
		if len(got.Recognised) != 0 {
			t.Errorf("recognised = %v, want none", got.Recognised)
		}
		if got.Fragment != "seni" {
			t.Errorf("fragment = %q", got.Fragment)
		}
	})

	t.Run("a whole phrase and a trailing fragment", func(t *testing.T) {
		got := ph.Parse("senior software engineer go")
		if len(got.Recognised) != 1 || got.Recognised[0].Text != "senior software engineer" {
			t.Fatalf("recognised = %v", got.Recognised)
		}
		if got.Fragment != "go" {
			t.Errorf("fragment = %q, want %q", got.Fragment, "go")
		}
	})

	// Longest match, not first: `senior` is not itself in this dictionary, but if a
	// shorter phrase ever prefixes a longer one, consuming the short one would strand
	// the rest of the longer one in the fragment.
	t.Run("takes the longest phrase it can", func(t *testing.T) {
		ph := phrases("data", "data engineer")
		got := ph.Parse("data engineer remote")
		if len(got.Recognised) != 1 || got.Recognised[0].Text != "data engineer" {
			t.Fatalf("recognised = %v", got.Recognised)
		}
		if got.Fragment != "remote" {
			t.Errorf("fragment = %q", got.Fragment)
		}
	})

	t.Run("a phrase with nothing after it leaves no fragment", func(t *testing.T) {
		got := ph.Parse("product owner ")
		if got.Fragment != "" {
			t.Errorf("fragment = %q, want empty", got.Fragment)
		}
	})

	// Recognition is EXACT. A mistyped phrase must fall through into the fragment,
	// where the index forgives it — being silently consumed as recognised would mean
	// the typo is never corrected and never completed.
	t.Run("a mistyped phrase is not consumed", func(t *testing.T) {
		// The misspellings are the point of the case, so they are built rather than
		// written: a spell-checking linter cannot tell a test fixture from a mistake,
		// and silencing it per line would hide a real typo in the next case.
		typo := "senior " + "sof" + "ware " + "engin" + "er"
		got := ph.Parse(typo)
		if len(got.Recognised) != 0 {
			t.Errorf("recognised = %v, want none", got.Recognised)
		}
		if got.Fragment != typo {
			t.Errorf("fragment = %q", got.Fragment)
		}
	})

	t.Run("several phrases in a row", func(t *testing.T) {
		ph := phrases("product owner", "java")
		got := ph.Parse("product owner java goo")
		if len(got.Recognised) != 2 {
			t.Fatalf("recognised = %v, want two", got.Recognised)
		}
		if got.Fragment != "goo" {
			t.Errorf("fragment = %q", got.Fragment)
		}
	})

	t.Run("normalises what it is given", func(t *testing.T) {
		got := ph.Parse("  PRODUCT OWNER  ")
		if len(got.Recognised) != 1 {
			t.Errorf("recognised = %v", got.Recognised)
		}
	})
}

// A query that has named a role is offered no second role — one job cannot be two
// roles, and the pair would filter to nothing. Skills are the exception: several
// skills narrow a search sensibly.
func TestParsed_ExcludedKinds(t *testing.T) {
	one := Parsed{Recognised: []Part{{Kind: KindCategory}}}
	if got := one.ExcludedKinds(); !slices.Contains(got, KindCategory) {
		t.Errorf("a named role must not be offered again, got %v", got)
	}

	skills := Parsed{Recognised: []Part{{Kind: KindSkill}}}
	if got := skills.ExcludedKinds(); slices.Contains(got, KindSkill) {
		t.Errorf("skills compose, got %v", got)
	}

	both := Parsed{Recognised: []Part{{Kind: KindCategory}, {Kind: KindCompany}}}
	got := both.ExcludedKinds()
	if !slices.Contains(got, KindCategory) || !slices.Contains(got, KindCompany) {
		t.Errorf("excluded = %v, want both", got)
	}
}

// Two kinds CAN spell the same phrase — `backend` is a skill AND a category — and the
// parse has to pick ONE, because that choice decides which facet a fully-typed phrase
// applies.
//
// First-writer-wins is not enough: the recognition set is loaded from the index, whose
// empty-query order is `searches:desc, jobs:desc`, so the winner would change as
// posting counts and demand move. The same query would apply a different filter on
// different days, and nothing would look broken. Hence an explicit precedence, tested
// with the input reversed.
func TestPhrases_DuplicatePhraseResolvesByKindNotByOrder(t *testing.T) {
	skill := Document{Kind: KindSkill, Slug: "backend", Text: "Backend", Jobs: 1}
	category := Document{Kind: KindCategory, Slug: "backend", Text: "Backend", Jobs: 1}

	for _, order := range [][]Document{
		{skill, category},
		{category, skill},
	} {
		// Trailing space: the precedence question is about a FINISHED word, and a word
		// still being typed is never recognised at all (see the trailing-word test).
		got := NewPhrases(order).Parse("backend ")
		if len(got.Recognised) != 1 {
			t.Fatalf("recognised = %v", got.Recognised)
		}
		// A technology named inside a job beats a department's name — the category is
		// the weakest reading of a phrase.
		if got.Recognised[0].Kind != KindSkill {
			t.Errorf("kind = %q, want skill regardless of input order", got.Recognised[0].Kind)
		}
	}
}

func TestPhrases_PrecedenceOrdersEveryKind(t *testing.T) {
	// A title beats a skill: a phrase somebody's posting is actually CALLED is a
	// stronger reading of the whole phrase than one technology named inside it.
	title := Document{Kind: KindTitle, Text: "Data Engineer", Jobs: 1}
	skill := Document{Kind: KindSkill, Slug: "data-engineer", Text: "Data Engineer", Jobs: 1}

	got := NewPhrases([]Document{skill, title}).Parse("data engineer ")
	if len(got.Recognised) != 1 || got.Recognised[0].Kind != KindTitle {
		t.Errorf("recognised = %v, want the title", got.Recognised)
	}
}

// The last word is being TYPED, so it must never be consumed as recognised — even
// when it happens to spell a phrase on its own.
//
// Observed on production: `senior software engineer go` consumed `go` as the skill
// (it is one), leaving nothing to complete, so the box offered "Senior Software
// Engineer Go Director" — three parts nobody was searching for. `go` there is the
// first two letters of `google`, and the visitor has not finished typing it.
//
// A word is only recognisable once a SPACE follows it. That is the whole signal, and
// it is the one the visitor gives.
func TestParse_TheWordStillBeingTypedIsNeverConsumed(t *testing.T) {
	ph := phrases("Senior Software Engineer", "Go")

	got := ph.Parse("senior software engineer go")
	if len(got.Recognised) != 1 {
		t.Fatalf("recognised = %v, want only the completed phrase", got.Recognised)
	}
	if got.Fragment != "go" {
		t.Errorf("fragment = %q, want the word still being typed", got.Fragment)
	}

	// The same word with a space after it IS finished, and is consumed.
	done := ph.Parse("senior software engineer go ")
	if len(done.Recognised) != 2 {
		t.Errorf("recognised = %v, want both once the word is finished", done.Recognised)
	}
}

// A single finished word is still a fragment until the space arrives, or the box would
// stop completing the moment a whole word matched.
func TestParse_OneUnfinishedWordIsAllFragment(t *testing.T) {
	got := phrases("Go").Parse("go")
	if len(got.Recognised) != 0 || got.Fragment != "go" {
		t.Errorf("recognised=%v fragment=%q", got.Recognised, got.Fragment)
	}
}

// If what has been typed so far is the BEGINNING of a longer phrase the dictionary
// knows, the visitor is still typing that phrase — so nothing is recognised yet.
//
// Observed on production: `java dev` recognised `java` as a finished title and
// completed `dev` against employers, offering "Java Dev.Pro" (49) where "Java
// Developer" (5,480) is the obvious answer. Refusing to consume a prefix of a longer
// phrase is what keeps the longer one reachable.
func TestParse_APrefixOfALongerPhraseIsStillBeingTyped(t *testing.T) {
	ph := phrases("Java", "Java Developer", "Senior Software Engineer")

	got := ph.Parse("java dev")
	if len(got.Recognised) != 0 {
		t.Errorf("recognised = %v, want none — this is the start of Java Developer", got.Recognised)
	}
	if got.Fragment != "java dev" {
		t.Errorf("fragment = %q, want the whole thing", got.Fragment)
	}

	// Not a prefix of anything longer, so the finished phrase IS recognised and the
	// trailing word gets completed. This is the composition the feature exists for.
	other := ph.Parse("senior software engineer go")
	if len(other.Recognised) != 1 || other.Fragment != "go" {
		t.Errorf("recognised=%v fragment=%q", other.Recognised, other.Fragment)
	}
}

// The exact phrase is not "still being typed" merely because a longer one starts with
// it — that is only true while the LAST word is unfinished.
func TestParse_AFinishedPhraseIsRecognisedEvenIfLongerOnesExist(t *testing.T) {
	ph := phrases("Java", "Java Developer")
	got := ph.Parse("java ")
	if len(got.Recognised) != 1 {
		t.Errorf("recognised = %v, want Java once the word is finished", got.Recognised)
	}
}
