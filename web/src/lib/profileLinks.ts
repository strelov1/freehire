// Sorting a candidate's flat list of profile links into the two the onboarding wizard has
// a named box for — LinkedIn and GitHub — and back again.
//
// The links live server-side as ONE untyped list (resume.Owned.Links), whether they were
// extracted from a CV or typed into the wizard. That is deliberate: naming them on the
// server would create a second and third place the same LinkedIn URL can live, with
// nothing keeping them in agreement. So the sorting is a presentation concern and lives
// here, next to the form that needs it.
//
// The host rule mirrors internal/candidate/linkedinprofile's Go matcher: an EXACT host
// match against a small set, never a suffix test and never a substring search.
// `linkedin.com.evil.example` ends in nothing this accepts, and that is the entire point —
// a lookalike host must not be handed the "this is your LinkedIn" box.

/** What a link is, as far as the wizard's two named fields are concerned. */
export type LinkKind = 'linkedin' | 'github' | 'other';

/** A flat link list sorted into the wizard's fields. `other` holds everything the
 *  classifier did not name, in its original order, so nothing is lost by a round trip. */
export interface ProfileLinks {
  linkedin: string;
  github: string;
  other: string[];
}

/** LinkedIn serves the same member profile from `linkedin.com`, `www.linkedin.com`, and a
 *  two-letter country subdomain. Matching that shape rather than listing every country is
 *  what the Go side does too. */
function isLinkedInHost(host: string): boolean {
  if (host === 'linkedin.com' || host === 'www.linkedin.com') return true;
  const sub = host.endsWith('.linkedin.com') ? host.slice(0, -'.linkedin.com'.length) : null;
  return sub !== null && /^[a-z]{2}$/.test(sub);
}

function isGitHubHost(host: string): boolean {
  return host === 'github.com' || host === 'www.github.com';
}

/** The URL a user pastes may arrive without a scheme (`linkedin.com/in/dana`), which is not
 *  a URL to the parser — it reads as a path. Returns null for anything that is not an
 *  http(s) address, so a `javascript:` string can never be classified as a profile. */
function parseHttpUrl(raw: string): URL | null {
  const input = raw.trim();
  if (input === '') return null;
  const withScheme = input.includes('://') ? input : `https://${input}`;
  let u: URL;
  try {
    u = new URL(withScheme);
  } catch {
    return null;
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return null;
  return u;
}

/** What kind of profile link this is. Anything unrecognised — including an unparseable
 *  string — is `other`, never a guess. */
export function classifyLink(raw: string): LinkKind {
  const u = parseHttpUrl(raw);
  if (!u) return 'other';
  const host = u.hostname.toLowerCase();

  // Only /in/<id> names a person. A LinkedIn company page in the "your LinkedIn" box would
  // be wrong in a way the candidate would not notice.
  if (isLinkedInHost(host)) {
    const segments = u.pathname.split('/').filter(Boolean);
    return segments.length >= 2 && segments[0] === 'in' ? 'linkedin' : 'other';
  }
  if (isGitHubHost(host)) {
    return u.pathname.split('/').filter(Boolean).length >= 1 ? 'github' : 'other';
  }
  return 'other';
}

/** Sort a stored link list into the wizard's fields. A repeated kind keeps its FIRST
 *  occurrence in the named field and the rest in `other` — two LinkedIn URLs is a CV that
 *  listed one twice or a candidate with two accounts, and dropping either would lose
 *  something they put there on purpose. */
export function splitProfileLinks(links: readonly string[]): ProfileLinks {
  const out: ProfileLinks = { linkedin: '', github: '', other: [] };
  for (const link of links) {
    const kind = classifyLink(link);
    if (kind === 'linkedin' && out.linkedin === '') out.linkedin = link;
    else if (kind === 'github' && out.github === '') out.github = link;
    else out.other.push(link);
  }
  return out;
}

/** Rebuild the flat list the server stores: the two named fields first (blank ones simply
 *  absent), then everything the classifier never named, untouched. */
export function mergeProfileLinks(links: ProfileLinks): string[] {
  const out: string[] = [];
  const linkedin = links.linkedin.trim();
  const github = links.github.trim();
  if (linkedin !== '') out.push(linkedin);
  if (github !== '') out.push(github);
  for (const other of links.other) {
    if (other.trim() !== '') out.push(other);
  }
  return out;
}
