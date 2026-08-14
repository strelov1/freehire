import { HIRE_ORIGIN } from './auth';

/** Hosts whose /jobs/<slug> pages we recognise as freehire job postings.
 *
 * freehire.dev stays on purpose: it is the pre-migration domain, and a user can
 * still land on one of its job pages from an old link or bookmark. This list
 * recognises what a tab might show, so it must outlive the canonical domain. */
const JOB_HOSTS = ['freehire.me', 'www.freehire.me', 'freehire.dev', 'localhost'];

/**
 * Returns the freehire job slug for a page URL, or null if the URL is not a
 * freehire job posting. Pure over its input.
 */
export function freehireSlugFromUrl(rawUrl: string): string | null {
  let u: URL;
  try {
    u = new URL(rawUrl);
  } catch {
    return null;
  }
  if (!JOB_HOSTS.includes(u.hostname)) return null;
  const m = u.pathname.match(/^\/jobs\/([^/]+)\/?$/);
  return m ? (m[1] ?? null) : null;
}

/** freehire's company-logo proxy URL for a company name (404s → placeholder).
 *
 * logo.freehire.me, matching the web app's COMPANY_LOGO_BASE. Both hosts serve,
 * so this was not broken — only divergent, and divergence is what leaves one of
 * two call sites behind on the next change. */
export function companyLogoUrl(company: string): string | null {
  const c = company.trim();
  return c ? `https://logo.freehire.me/${encodeURIComponent(c)}` : null;
}

/** The slice of a freehire job the card renders. Facet fields mirror
 *  internal/jobview.Job's wire shape (see internal/jobview/AGENTS.md) — the
 *  handler already serves them on GET /jobs/{slug}, so no backend change was
 *  needed to read them here, only to widen this projection. */
export interface FreehireJob {
  public_slug: string;
  title: string;
  company: string;
  location: string;
  posted_at?: string | null;
  work_mode?: string;
  regions?: string[];
  countries?: string[];
  enrichment?: {
    category?: string;
    seniority?: string;
  };
}

/** Deterministic skill-coverage match against the signed-in user's profile. */
export interface JobMatch {
  coverage_percent: number;
  total: number;
  matched: string[];
  adjacent: { skill: string }[];
  missing: string[];
}

/**
 * What a failed call tells the user. hire answers `{"error": "<why>"}`, and that
 * sentence is the whole diagnosis: `/me/autofill/run` alone returns 409 for three
 * unrelated states — no browser attached, no form on the page, no model
 * configured — so a bare status collapses them into one line nobody can act on.
 * The status still travels for a bug report; the path only stands in when the
 * body says nothing, as a proxy's HTML error page does. Pure over its input.
 */
export function apiErrorMessage(path: string, status: number, body: string): string {
  const reason = serverReason(body);
  return reason ? `${reason} (HTTP ${status})` : `${path} → HTTP ${status}`;
}

function serverReason(body: string): string {
  try {
    const { error } = JSON.parse(body) as { error?: unknown };
    return typeof error === 'string' ? error.trim() : '';
  } catch {
    return '';
  }
}

/** Reads the body once: as the payload when the call succeeded, as the reason when it did not. */
async function unwrap<T>(path: string, res: Response): Promise<T> {
  const body = await res.text();
  if (!res.ok) throw new Error(apiErrorMessage(path, res.status, body));
  return (JSON.parse(body) as { data: T }).data;
}

async function getData<T>(path: string, token: string): Promise<T> {
  const res = await fetch(`${HIRE_ORIGIN}${path}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  return unwrap<T>(path, res);
}

async function postData<T>(path: string, body: unknown, token: string): Promise<T> {
  const res = await fetch(`${HIRE_ORIGIN}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify(body),
  });
  return unwrap<T>(path, res);
}

async function deleteData<T>(path: string, token: string): Promise<T> {
  const res = await fetch(`${HIRE_ORIGIN}${path}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
  return unwrap<T>(path, res);
}

export function getJob(slug: string, token: string): Promise<FreehireJob> {
  return getData<FreehireJob>(`/api/v1/jobs/${encodeURIComponent(slug)}`, token);
}

export function getMatch(slug: string, token: string): Promise<JobMatch> {
  return getData<JobMatch>(`/api/v1/jobs/${encodeURIComponent(slug)}/match`, token);
}

/**
 * Ad-hoc match for a job posting scraped off any page — no catalog job needed.
 * The server extracts skills from title+text and scores them against the profile.
 */
export function getMatchText(title: string, text: string, token: string): Promise<JobMatch> {
  return postData<JobMatch>('/api/v1/me/match-text', { title, text }, token);
}

/** The signed-in user's interaction with a job — only the field the Save button reads. */
export interface JobInteraction {
  saved_at: string | null;
}

export function saveJob(slug: string, token: string): Promise<JobInteraction> {
  return postData<JobInteraction>(`/api/v1/jobs/${encodeURIComponent(slug)}/save`, {}, token);
}

export function unsaveJob(slug: string, token: string): Promise<JobInteraction> {
  return deleteData<JobInteraction>(`/api/v1/jobs/${encodeURIComponent(slug)}/save`, token);
}

/** A cached AI fit analysis — only the fields the compact card reads. Never computes
 *  inline: this is a read of whatever the full-page analysis last cached, same
 *  contract as web's MatchSummary.svelte. */
export interface MatchAnalysisSummary {
  overall_score: number;
  verdict: string;
  gaps: string[];
}

export interface MatchAnalysisCredits {
  remaining: number;
  resets_at: string;
}

export interface MatchAnalysisResponse {
  has_cv: boolean;
  analysis: MatchAnalysisSummary | null;
  credits: MatchAnalysisCredits | null;
}

export function getMatchAnalysis(slug: string, token: string): Promise<MatchAnalysisResponse> {
  return getData<MatchAnalysisResponse>(`/api/v1/jobs/${encodeURIComponent(slug)}/match-analysis`, token);
}

/** Canonical autofill fields freehire assembles from the user's CV + account. */
export interface AutofillProfile {
  full_name: string;
  first_name: string;
  last_name: string;
  email: string;
  phone: string;
  location: string;
  linkedin: string;
  github: string;
  portfolio: string;
}

export function getAutofillProfile(token: string): Promise<AutofillProfile> {
  return getData<AutofillProfile>('/api/v1/me/autofill-profile', token);
}

/** What the agent did to the form it was pointed at. */
export interface AutofillReport {
  filled: string[];
  /** Custom-widget comboboxes — not fillable yet, reported rather than corrupted. */
  deferred: string[];
  /** Fields the agent found no basis for in the profile. */
  unmapped: string[];
}

/**
 * Asks freehire's agent to fill the form on the page the panel is looking at. The
 * agent drives this extension back over the browser-tool wire, so the panel's
 * ToolChannel has to be attached before this is called.
 */
export function runAgentAutofill(token: string): Promise<AutofillReport> {
  return postData<AutofillReport>('/api/v1/me/autofill/run', {}, token);
}

/** What the server did with a page we offered it.
 *  - `found`    the catalog already carried the posting
 *  - `tracked`  imported, and freehire already crawls this company's board
 *  - `imported` imported, and the board behind it is now queued for onboarding
 *  - `queued`   nothing could read the page, so the link went to manual triage */
export type ResolveStatus = 'found' | 'tracked' | 'imported' | 'queued';

export interface ResolvedPage {
  public_slug: string | null;
  status: ResolveStatus;
  /** Set only for `tracked`: the company freehire already covers. */
  company_slug?: string;
}

/**
 * Offers the page to freehire: the server answers with the catalog posting for it,
 * importing the vacancy when it can read the page and queueing the link for a
 * maintainer when it cannot. Authenticated — it makes the server fetch the URL.
 */
export function resolveJob(url: string, token: string): Promise<ResolvedPage> {
  return postData<ResolvedPage>('/api/v1/jobs/resolve', { url, surface: 'extension' }, token);
}

/** The sentence the panel shows for each outcome. Pure. */
export function resolveNotice(status: ResolveStatus): string {
  switch (status) {
    case 'imported':
      return '✓ Added to freehire — here is your match.';
    case 'tracked':
      return '✓ Added — freehire already follows this company, so its other roles will show up too.';
    case 'found':
      return 'freehire already had this posting — here is your match.';
    case 'queued':
      return "We could not read this page, so we'll have a look at the link.";
    default:
      return 'Sent this page to freehire.';
  }
}

/**
 * Resolves the page's URL to a freehire catalog slug, or null when the posting
 * is not one we carry (or is on an ATS the server cannot yet read a job id
 * from). The server matches on the posting's own identity in the URL; it used to
 * take a company and a guessed title, which was both unreliable and slow enough
 * to time out.
 */
export async function findJob(url: string, token: string): Promise<string | null> {
  const found = await getData<{ public_slug: string } | null>(
    `/api/v1/jobs/find?url=${encodeURIComponent(url)}`,
    token,
  );
  return found?.public_slug ?? null;
}
