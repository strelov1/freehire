// The "Filtering jobs" section of the API reference, built from filters.ts data by
// gen-api-docs.mjs into each document's info.description (and so into docs/API*.md).
//
// Returns an array of Markdown lines (not a joined string) so a caller can
// `out.push(...renderFilterSectionLines(filters))` alongside its own lines.

function filterTableLines(rows) {
  const lines = ['| Param | Filter | Values |', '| --- | --- | --- |'];
  for (const f of rows) lines.push(`| \`${f.param}\` | ${f.label} | ${f.values} |`);
  return lines;
}

export function renderFilterSectionLines(filters) {
  const { FILTER_FACETS, FILTER_EXTRAS, FILTER_MODIFIERS, RECIPES } = filters;
  const out = [];
  out.push('## Filtering jobs');
  out.push('');
  out.push(
    'Except where noted below, these parameters apply to `GET /jobs/search` and `GET /jobs/facets`. Combine ' +
      'any of them with full-text `q`.',
  );
  out.push('');
  for (const m of FILTER_MODIFIERS) out.push(`- ${m}`);
  out.push('');
  out.push('### Facets');
  out.push('');
  out.push('Every facet below supports repeat-OR, `_mode=and`, and `_exclude` as described above.');
  out.push('');
  out.push(...filterTableLines(FILTER_FACETS));
  out.push('');
  out.push('### Numeric & boolean filters');
  out.push('');
  out.push(...filterTableLines(FILTER_EXTRAS));
  out.push('');
  out.push('### Recipes');
  out.push('');
  for (const r of RECIPES) out.push(`- **${r.title}** — \`${r.query}\``);
  return out;
}
