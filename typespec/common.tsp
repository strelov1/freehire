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
