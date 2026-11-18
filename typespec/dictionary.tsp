import "@typespec/http";

using Http;

namespace Freehire;

model DictionaryGap {
  /** `specialization`, `skill` or `industry`; anything else is a 400. */
  kind?: string;

  term?: string;
}

model DictionaryGapRecorded {
  /** How many people have asked for this term. */
  request_count: int64;

  status: string;
}

/**
 * Asks for a term the dictionary does not resolve. 409 when it already does. Session
 * cookie only.
 */
@tag("Dictionary")
@route("/dictionary/gaps")
@post
op recordDictionaryGap(@body body: DictionaryGap): Data<DictionaryGapRecorded> | Problem;
