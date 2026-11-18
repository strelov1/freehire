package main

// stopWords never carry a role: employment type, schedule notation and posting boilerplate.
// No word group may contain one, anywhere.
//
// Curated, never inferred, deliberately conservative (as internal/dict/classify): a word here
// silently removes every group containing it, so "home" would cost "home health aide".
// Words like "shift", "night", "day" and "travel" are deliberately NOT listed, since they anchor
// real roles; noise in the report is cheap to skip while a hidden cluster is invisible. When
// unsure, leave the word out. A test checks no entry is a token of a classify non-tech term,
// but it cannot see the unknown families the miner exists to find; re-run with an emptied
// list once per campaign to see what the list hides.
var stopWords = []string{
	// Pronouns — never part of a role
	"you", "our", "your", "we",

	// Employment type and work arrangement — a job attribute, not a role. Schedule
	// words that also anchor roles are deliberately absent; see the note above.
	"full", "part", "time", "hour", "hours", "week", "weekly", "weekday",
	"weekdays", "weekend", "weekends", "prn", "temp", "temporary", "permanent",
	"contract", "casual", "seasonal", "remote", "onsite", "hybrid",

	// Posting boilerplate — the words a board wraps around the role
	"new", "needed", "hiring", "urgent", "immediate", "immediately", "now", "join",
	"job", "jobs", "position", "positions", "opening", "openings", "opportunity",
	"career", "careers", "apply", "req", "experience", "required", "sign", "bonus",
	"pay", "salary", "rate", "level", "based", "genders",
}

// connectors are function words that belong inside a role phrase but never at its edge:
// "operador de caixa" is a role, "analista de" a fragment. As stop words they would make the
// Portuguese and Spanish tail unmineable; as ordinary words they would fill the ranking with
// fragments.
//
// A group is kept only when no connector sits at either end, so a two-word group cannot contain
// one; that is why the miner also builds three-word groups.
var connectors = []string{
	// English. "at" and "and" are deliberately absent: they bridge junk trigrams out of
	// location suffixes ("driver at lih").
	"of", "in", "to", "for", "or", "the", "with", "per", "from", "a", "an",
	// Portuguese and Spanish
	"de", "da", "do", "dos", "das", "em", "para", "com", "del", "la", "el", "los",
	"las", "por", "que", "y",
	// German. "für" keeps its umlaut through the Unicode tokenizer; the ASCII spelling would not match.
	"der", "die", "und", "für",
	// Russian
	"по", "на", "для", "или", "под", "и", "с", "в", "от", "к",
	// Danish and Norwegian ("for" is already listed above).
	"til", "med", "hos",
}

// A language missing here loses its roles from the report entirely: its short prepositions are
// dropped by the query's three-character rule and cannot be bridged ("X di Y", "X w Y"). Add a
// language's connectors before mining a market that uses it.
