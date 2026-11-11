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

/** A page of a list, with what the filter dropped. */
model ListMeta {
  total: int64;
  limit: int32;
  offset: int32;

  /** Absent when every param was read. */
  ignored_params?: IgnoredParam[];
}

/** A list: `{"data": [...], "meta": {...}}`. */
model List<T> {
  data: T[];
  meta: ListMeta;
}

/** The meta of a single-item answer, sent only to carry a warning. */
model IgnoredMeta {
  ignored_params?: IgnoredParam[];
}

/** A single item that read query params: `{"data": ...}`, plus `meta` when one was dropped. */
model DataWithIgnored<T> {
  data: T;
  meta?: IgnoredMeta;
}
