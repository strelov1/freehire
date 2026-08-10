// URLSearchParams.toString() percent-encodes a literal comma to %2C, per the
// application/x-www-form-urlencoded serializer it implements. A comma is not a
// delimiter for URLSearchParams (a value containing one parses back identically
// whether escaped or literal) and is valid unescaped in a URI query component
// (RFC 3986 sub-delims), so decoding it back keeps a compact comma-joined value
// (see facetModel.ts's filtersToParams) human-readable in the address bar
// without changing what the string parses back to.

/** Serialize URLSearchParams for display/storage, undoing the %2C encoding of
 *  literal commas. */
export function toSearchString(params: URLSearchParams): string {
  return params.toString().replace(/%2C/g, ',');
}
