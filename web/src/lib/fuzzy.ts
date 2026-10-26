// Typo-tolerant matching for facet-option filtering (the role picker's search box).
// The option set is small and already client-side, so this runs locally per
// keystroke — no backend round-trip. It tolerates common typos and adjacent
// transpositions ("sineor" → "senior") but not heavy manglings, to avoid noise.

// osaDistance is the Optimal String Alignment distance: Levenshtein plus adjacent
// transpositions counted as one edit (so "backedn" → "backend" is distance 1). It
// is not the full Damerau distance (no overlapping transpositions), which is more
// than enough for short single words.
function osaDistance(a: string, b: string): number {
  const m = a.length;
  const n = b.length;
  if (m === 0) return n;
  if (n === 0) return m;
  // Flat (m+1)×(n+1) matrix in a typed array (row-major, stride w). A TypedArray
  // element type is `number`, so it sidesteps noUncheckedIndexedAccess unlike a
  // nested number[][].
  const w = n + 1;
  const d = new Int32Array((m + 1) * w);
  // Every read below is provably in-bounds; the assertion just drops the
  // `| undefined` that noUncheckedIndexedAccess adds to indexed access.
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- hot per-keystroke loop; a real check would run per cell
  const at = (k: number): number => d[k]!;
  for (let i = 0; i <= m; i++) d[i * w] = i;
  for (let j = 0; j <= n; j++) d[j] = j;
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      let v = Math.min(at((i - 1) * w + j) + 1, at(i * w + j - 1) + 1, at((i - 1) * w + j - 1) + cost);
      if (i > 1 && j > 1 && a[i - 1] === b[j - 2] && a[i - 2] === b[j - 1]) {
        v = Math.min(v, at((i - 2) * w + j - 2) + 1);
      }
      d[i * w + j] = v;
    }
  }
  return at(m * w + n);
}

// maxEdits scales the allowed edit distance with the query token's length: short
// tokens must be near-exact (a stray edit on a 3-letter word matches too much),
// longer tokens tolerate more.
function maxEdits(len: number): number {
  if (len <= 2) return 0;
  if (len <= 4) return 1;
  if (len <= 6) return 2;
  return 3;
}

// fuzzyMatch reports whether `query` loosely matches `text`. An empty query always
// matches. A contiguous substring is a fast exact hit. Otherwise every
// whitespace-separated query token must match some word of the text — as a
// substring or within maxEdits — so "sineor back" still finds "Senior Backend
// Engineer".
export function highlightParts(text: string, query: string): { text: string; match: boolean }[] {
  const q = query.trim().toLowerCase();
  if (q === '' || text === '') return [{ text, match: false }];
  const spans = q
    .split(/\s+/)
    .map((token) => closestSpan(text, token))
    .filter((span): span is [number, number] => span != null)
    .sort((a, b) => a[0] - b[0] || b[1] - a[1]);
  const taken: [number, number][] = [];
  for (const span of spans) {
    if (taken.some(([start, end]) => span[0] < end && span[1] > start)) continue;
    taken.push(span);
  }
  taken.sort((a, b) => a[0] - b[0]);
  if (taken.length === 0) return [{ text, match: false }];
  const parts: { text: string; match: boolean }[] = [];
  let cursor = 0;
  for (const [start, end] of taken) {
    if (start > cursor) parts.push({ text: text.slice(cursor, start), match: false });
    parts.push({ text: text.slice(start, end), match: true });
    cursor = end;
  }
  if (cursor < text.length) parts.push({ text: text.slice(cursor), match: false });
  return parts;
}

function closestSpan(text: string, token: string): [number, number] | null {
  const t = text.toLowerCase();
  const exact = t.indexOf(token);
  if (exact >= 0) return [exact, exact + token.length];
  const budget = maxEdits(token.length);
  if (budget === 0) return null;
  let best: { i: number; n: number; d: number } | null = null;
  const minLen = Math.max(1, token.length - budget);
  const maxLen = token.length + budget;
  for (let n = minLen; n <= maxLen; n++) {
    for (let i = 0; i + n <= t.length; i++) {
      const window = t.slice(i, i + n);
      if (window.includes(' ')) continue;
      const d = osaDistance(token, window);
      if (d > budget) continue;
      if (best == null || d < best.d || (d === best.d && (n > best.n || (n === best.n && i < best.i)))) {
        best = { i, n, d };
      }
    }
  }
  return best ? [best.i, best.i + best.n] : null;
}

export function fuzzyMatch(text: string, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (q === '') return true;
  const t = text.toLowerCase();
  if (t.includes(q)) return true;
  const words = t.split(/\s+/).filter(Boolean);
  return q.split(/\s+/).every((token) => {
    const budget = maxEdits(token.length);
    return words.some((w) => w.includes(token) || osaDistance(token, w) <= budget);
  });
}
