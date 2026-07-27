package main

// The miner ranks word groups drawn from job titles, and two classes of word have
// to be handled differently or the ranking fills with noise.
//
// stopWords never carry a role — employment type, schedule notation and posting
// boilerplate. A measurement against prod put "m w"/"w d"/"h f" (the shrapnel of
// M-W-F schedules) and "part time" above every real role, and they alone accounted
// for most of the apparent coverage. No word group may contain one, anywhere.
//
// The doctrine matches internal/classify: curated, never inferred, deliberately
// conservative. A word here silently removes every group containing it, so a term
// that is part of a real role phrase would hide that whole family from mining
// rather than merely filter it — "home" would cost "home health aide", "call" would
// cost "call center".
//
// A test asserts no entry is a token of any classify non-tech term, but note what
// that guard can and cannot do: every family already in that dictionary is by
// construction already classified, so it is absent from the unclassified population
// this miner reads. The guard protects the families that need it least; the ones at
// risk are the unknown families the miner exists to discover, which no test can see.
// That is why words like "shift", "night", "day" and "travel" are deliberately NOT
// listed despite being schedule words — they anchor real roles ("shift supervisor",
// "night auditor", "day porter", "travel nurse"), and noise in the report is cheap
// for an operator to skip while a hidden cluster is invisible. The asymmetry decides
// every borderline case: when unsure, leave the word out. Re-running the miner with
// an emptied stop list once per campaign is the only way to see what the list hides.
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

// connectors are function words that belong INSIDE a role phrase but never at its
// edge. Romance-language titles make this distinction load-bearing: in Portuguese
// and Spanish the preposition is part of the role name — "operador de caixa",
// "auxiliar de limpeza", "técnico de enfermagem" are all in the classify non-tech
// dictionary — while a pair ending or beginning in the same word ("analista de",
// "banco de") is a fragment. Treating them as ordinary stop words would make the
// whole Portuguese and Spanish tail of the catalogue unmineable; treating them as
// ordinary words would fill the ranking with fragments.
//
// So a group is kept only when no connector sits at either end. A two-word group
// therefore cannot contain one at all, which is why the miner also builds
// three-word groups: they are the shortest unit that can carry a connector.
var connectors = []string{
	// English. "at" and "and" are deliberately absent: they occur inside no English role
	// name we need to reproduce, while a prod run showed them bridging junk trigrams —
	// "driver at lih", "home and clinic" — out of location suffixes.
	"of", "in", "to", "for", "or", "the", "with", "per", "from", "a", "an",
	// Portuguese and Spanish
	"de", "da", "do", "dos", "das", "em", "para", "com", "del", "la", "el", "los",
	"las", "por", "que", "y",
	// German. "für" carries its umlaut through the Unicode tokenizer, so the ASCII
	// spelling would never match.
	"der", "die", "und", "für",
	// Russian
	"по", "на", "для", "или", "под", "и", "с", "в", "от", "к",
	// Danish and Norwegian — a prod run surfaced "søges til" ("wanted for") in the
	// top 100, which is a posting phrase, not a role. Danish "for" is spelled like
	// the English one and is already listed above.
	"til", "med", "hos",
}

// A language whose function words are absent here fails in a particular way worth
// knowing: its short prepositions are dropped by the query's three-character rule
// AND cannot be bridged, so "X di Y" (Italian) or "X w Y" (Polish) yields only the
// pair "X Y"... which never forms, because the words are not adjacent. Those roles
// vanish from the report entirely rather than appearing as noise. Add a language's
// connectors before mining a market that uses it.
