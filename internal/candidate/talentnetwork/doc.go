// Package talentnetwork is the public catalogue of candidates who asked to be found:
// the membership handle, the projection that decides what a stranger may see, and the
// filtered list served over it.
//
// # The one rule
//
// Everything this package publishes is a value a DICTIONARY resolved, a number, or a
// date; the one exception is CandidateRole.Title (see card.go). This is stricter than
// resumeextract.Anonymous, which masks only the current employer column while the name
// usually sits in the prose beside it, and is fine for a link handed to one person but not
// for a page a crawler reads. A whitelist also fails safe: a field the extraction contract
// later grows stays withheld until someone adds it deliberately.
//
// # What it costs
//
// Languages are absent (no dictionary this block can reach). Education carries only the
// degree level and year, certifications only canonical names.
//
// # Where it sits
//
// Block `candidate`, layer 4. It reads `dict` and `platform`, nothing above. The
// recruiter-facing half (names and employers) will live in `engage`, layer 7, and import
// this; the layering guard prevents the reverse.
package talentnetwork
