/** The text a list's result-count live region speaks. Empty while the list is loading
 *  (or failed — the error state speaks for itself), the count once it has one, and
 *  "No matching …" only when the caller has said the list settled on nothing: `total`
 *  alone is null for both "loading" and "zero results", so without `settled` nothing
 *  is claimed. `atLeast` says the count is a minimum, which is spoken as one. */
export function resultCountMessage(
  total: number | null,
  unit: string,
  settled: boolean | undefined,
  atLeast = false,
): string {
  if (settled === false) return '';
  if (total !== null) return `${atLeast ? 'At least ' : ''}${total.toLocaleString()} ${unit}`;
  return settled ? `No matching ${unit}` : '';
}
