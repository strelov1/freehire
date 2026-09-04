// Telling a pasted LINK apart from a typed QUERY, in the search box.
//
// The box is one input serving two intents now. Almost everything typed into it is a
// query; occasionally somebody drops in the URL of a vacancy they found elsewhere and
// wants to know whether we carry it. Running that URL as a full-text search finds
// nothing, every time — so the box has to recognise it and offer the other thing.
//
// Pure by design: no Svelte, no DOM, no network. What HAPPENS to a recognised link
// lives in linkIntake.svelte.ts; this module only answers "is this a link, and is it
// one of ours".

/** A link recognised in the search box.
 *
 *  `ownSlug` is set only when the URL is one of OUR OWN posting pages. Pasting a
 *  freehire link back into freehire is a thing people do — from a chat, from their own
 *  bookmarks — and there is nothing to look up in that case: the slug is in the path,
 *  so the box can navigate straight there without asking the API whether we carry a
 *  posting we are literally serving. */
export interface PastedJobLink {
  url: string;
  ownSlug: string | null;
}

/** A bare host followed by a path: `boards.greenhouse.io/acme/jobs/123`.
 *
 *  The PATH is the load-bearing part. A vacancy always has one, and requiring it is what
 *  keeps `node.js`, `react.dev` and `express.js` — all of them valid hostnames, all of
 *  them things people type into this box meaning "find me jobs" — on the search side of
 *  the line. A bare domain with no path is a company, and searching for the company is
 *  the more useful of the two answers anyway.
 *
 *  The last label is letters only, so `1.5/10` and version numbers do not read as hosts. */
const HOST_WITH_PATH = /^[a-z0-9-]+(?:\.[a-z0-9-]+)*\.[a-z]{2,}(?::\d+)?\/\S*$/i;

/** Does this text name a web address? Returns the absolute URL to hand on, or null when
 *  it is an ordinary query.
 *
 *  Deliberately conservative in one direction only: a paste that is not recognised still
 *  runs as a search, which finds nothing and costs a retry. A QUERY misread as a link
 *  would replace the panel with a row offering to import it — the box would look broken
 *  for a word somebody typed on purpose. So the cheap mistake is the one this makes. */
export function linkInText(text: string): string | null {
  const trimmed = text.trim();
  // Whitespace anywhere means a phrase. No URL survives a space, and most of what this
  // box receives is two or three words.
  if (trimmed === '' || /\s/.test(trimmed)) return null;

  // An explicit scheme is somebody's own statement that this is an address; only http(s)
  // is worth handing to the intake, which refuses anything else regardless.
  if (/^https?:\/\//i.test(trimmed)) return hasHost(trimmed) ? trimmed : null;

  // Browsers drop the scheme when you copy from the address bar, so a paste arrives bare
  // more often than not.
  if (!HOST_WITH_PATH.test(trimmed)) return null;
  const withScheme = `https://${trimmed}`;
  return hasHost(withScheme) ? withScheme : null;
}

/** Whether the URL parses and names a host — `https://` alone does neither. */
function hasHost(raw: string): boolean {
  try {
    return new URL(raw).host !== '';
  } catch {
    return false;
  }
}

/** Recognise a pasted link in the search box, and note when it is one of our own.
 *
 *  `origin` is the page's own origin, so a link is "ours" by the same rule in
 *  production, in a preview deploy and on localhost — rather than by a hostname
 *  hardcoded here, which would be right in exactly one of those three. */
export function pastedJobLink(text: string, origin: string): PastedJobLink | null {
  const url = linkInText(text);
  if (url === null) return null;

  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return null;
  }

  return { url, ownSlug: ownPostingSlug(parsed, origin) };
}

/** The posting slug of one of our own job pages, or null for anything else.
 *
 *  Matched against the running origin rather than a hostname list: the same paste has
 *  to behave the same way wherever the app is served from. A link to our own site that
 *  is NOT a posting (a company page, the feed) returns null and takes the ordinary
 *  path — the intake will find it uninteresting, which is the honest answer. */
function ownPostingSlug(url: URL, origin: string): string | null {
  let here: URL;
  try {
    here = new URL(origin);
  } catch {
    return null;
  }
  if (url.host !== here.host) return null;

  // /jobs/<slug>, and nothing deeper: /jobs/<slug>/apply is a different page, and
  // sending it to the posting would quietly drop what the visitor asked for.
  const [section, slug, ...rest] = url.pathname.split('/').filter((p) => p !== '');
  if (section !== 'jobs' || !slug || rest.length > 0) return null;

  // A lone `%` is a legal path and an illegal escape, so decoding it THROWS — and this
  // runs inside a $derived on every keystroke in the header, where a throw takes the
  // whole page down rather than the one row. An undecodable segment is handed back as
  // it stands: it is not a slug we issued, so it lands on our own 404 instead of a
  // blank screen.
  try {
    return decodeURIComponent(slug);
  } catch {
    return slug;
  }
}
