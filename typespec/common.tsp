import "@typespec/openapi";

using TypeSpec.OpenAPI;

namespace Freehire;

/** A single item: `{"data": ...}`. */
model Data<T> {
  data: T;
}

/**
 * Every refusal, whatever its status: `{"error": msg}`. `code` is set only where a
 * client branches on the reason (the auth flows).
 */
@error
model Problem {
  error: string;
  code?: string;
}

/**
 * A query param the endpoint did not read: unknown, or known but unreadable
 * (`min_years=lots`). Either way the answer is wider than the caller asked for.
 */
model IgnoredParam {
  param: string;
  did_you_mean?: string;
}

/**
 * Present only when a closed-vocabulary facet (or its `_exclude` form) carried a value
 * outside its vocabulary — keyed by the parameter as sent, each listing the unknown values.
 * On a facet the value is still applied and matches no posting: alone it empties the
 * answer, beside other values it adds nothing to their OR, in `_exclude` it excludes
 * nothing. A boolean or modifier param reads it its own way (`requires_clearance=flase`
 * reads as true, `visa_sponsorship=yes` filters nothing), so the answer may be wider.
 */
// ogen would encode a map in random key order, so the handlers write this as raw JSON
// (contractIgnoredValues sorts it): scripts/openapi-ogen-input.mjs hands ogen an untyped
// schema for a model carrying `x-raw-json`.
@extension("x-raw-json", true)
model IgnoredValues is Record<string[]>;

/** Where a page sits in the whole list. */
model PageMeta {
  total: int64;
  limit: int64;
  offset: int64;
}

/** A page of a filtered list, with what the filter dropped. */
model ListMeta {
  ...PageMeta;

  /** Absent when every param was read. */
  ignored_params?: IgnoredParam[];
  ignored_values?: IgnoredValues;

  /** True when `total` counts only what was read, not the whole match. */
  total_is_lower_bound?: boolean;
}

/** A list: `{"data": [...], "meta": {...}}`. */
model List<T> {
  data: T[];
  meta: ListMeta;
}

/** The meta of a single-item answer, sent only to carry a warning. */
model IgnoredMeta {
  ignored_params?: IgnoredParam[];
  ignored_values?: IgnoredValues;
}

/** A single item that read query params: `{"data": ...}`, plus `meta` when one was dropped. */
model DataWithIgnored<T> {
  data: T;
  meta?: IgnoredMeta;
}
