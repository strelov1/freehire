// Package talentnetwork is the public catalogue of candidates who asked to be found:
// the membership handle, the projection that decides what a stranger may see, and the
// filtered list served over it.
//
// # The one rule
//
// Everything this package publishes is a value a DICTIONARY resolved, a number, or a
// date. Nothing a candidate typed reaches a public response. That is stricter than
// masking the fields which identify somebody, and the difference is the reason
// ProjectCard exists rather than reusing resumeextract.Anonymous: Anonymous replaces the
// current role's employer column and leaves the rest alone, which is right for a link
// handed to one person and wrong for a page a crawler reads. The employer's name is
// usually sitting in the prose beside the column that was masked.
//
// A whitelist of dictionary-resolved values also fails in the safe direction. Masking
// named fields means the next field the extraction contract grows is published by
// default; a whitelist means it is withheld until somebody adds it deliberately.
//
// # What it costs
//
// Languages, certifications and education are absent, because all three are free text
// and no dictionary this block can reach resolves them. vocab.EducationLevelValues
// exists, but the text-to-level resolver lives in internal/job/jobfacts — block `job`,
// layer 5 — which `candidate` may not import. Publishing them means moving that
// dictionary down into `dict` first.
//
// # Where it sits
//
// Block `candidate`, layer 4. It reads `dict` (classify, skilltag) and `platform` (db),
// and nothing above. The recruiter-facing half of this product — approved accounts that
// see names and employers — will live in `engage`, layer 7, and will import this; the
// reverse is what the layering guard exists to prevent.
package talentnetwork
