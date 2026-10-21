/** A metro facet value is "<Name> (<CC>)" (internal/dict/location, metros.tsv). The pane
 *  shows "<Name> area" — unless the name already says it is an area. */
export function metroLabel(value: string): string {
  const name = value.replace(/ \([A-Z]{2}\)$/, '');
  return /\barea$/i.test(name) ? name : `${name} area`;
}
