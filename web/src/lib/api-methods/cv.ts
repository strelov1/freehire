import type { Http } from '../api-core';
import type { PhotoMeta } from '../types';
import type {
  CoverLetterView,
  CvAppearanceDefaults,
  CvAtsDelta,
  CvBaseItem,
  CvFont,
  CvJobMatch,
  CvMeta,
  CvRecord,
  CvTailoredItem,
  CvTemplate,
  CvTracerLink,
  JdResolveInput,
  TailorResult,
  UpdateCvInput,
} from '../cv';
import type { RevisionView } from '#lib/generated/contracts.js';
import { ApiError, jsonBody, PHOTO_MAX_BYTES, PHOTO_MAX_MB } from '../api-core';

export function cvApi({ call, requestData, baseUrl }: Http) {
  // --- CVs and tailoring (open to every signed-in user; the plan meters the AI spend) ---

  /** The caller's headshot: whether the feature is configured at all (`enabled`), whether
   *  one is stored, and when it was uploaded. Always 200 — "no photo yet" and "storage is
   *  off" are both states the profile renders. */
  async function getPhoto(): Promise<PhotoMeta> {
    return requestData<PhotoMeta>('/api/v1/me/photo');
  }

  /** Store (or replace) the caller's headshot. The server normalizes it to a square JPEG,
   *  so an image it cannot decode comes back as a 400 and nothing is stored. */
  async function putPhoto(file: File): Promise<PhotoMeta> {
    if (file.size > PHOTO_MAX_BYTES) {
      throw new ApiError(413, `This image is larger than ${PHOTO_MAX_MB} MB. Pick a smaller photo and try again.`);
    }
    const form = new FormData();
    form.append('file', file);
    return requestData<PhotoMeta>('/api/v1/me/photo', { method: 'PUT', body: form });
  }

  /** Remove the caller's headshot (object + pointer). */
  async function deletePhoto(): Promise<void> {
    await call('/api/v1/me/photo', { method: 'DELETE' });
  }

  /** List the available CV templates (id, label, style, photo) for the gallery. */
  async function listCvTemplates(): Promise<CvTemplate[]> {
    return requestData<CvTemplate[]>('/api/v1/cv-templates');
  }

  /** List the typefaces a CV may use (id, label, note, css) for the font picker. */
  async function listCvFonts(): Promise<CvFont[]> {
    return requestData<CvFont[]>('/api/v1/cv-fonts');
  }

  /** Switch a CV's template only (title + document untouched). */
  async function setCvTemplate(id: string, templateId: string): Promise<void> {
    await call(`/api/v1/me/cvs/${id}/template`, jsonBody('PUT', { template_id: templateId }));
  }

  /** The caller's saved defaults for a NEW CV's template/typography/margins — or the system's
   *  standard defaults when they have never saved any. */
  async function getCvAppearanceDefaults(): Promise<CvAppearanceDefaults> {
    return requestData<CvAppearanceDefaults>('/api/v1/me/cv-appearance-defaults');
  }

  /** Replace the caller's saved CV appearance defaults wholesale. Only seeds CVs created
   *  afterward — never touches an existing CV. */
  async function setCvAppearanceDefaults(defaults: CvAppearanceDefaults): Promise<CvAppearanceDefaults> {
    return requestData<CvAppearanceDefaults>('/api/v1/me/cv-appearance-defaults', jsonBody('PUT', defaults));
  }

  /** List the caller's TAILORED CVs (the re-open list): each with its vacancy slug + bound
   *  agent session, newest edit first. */
  async function listCvs(): Promise<CvTailoredItem[]> {
    return requestData<CvTailoredItem[]>('/api/v1/me/cvs');
  }

  /** The caller's BASE (non-tailored) CVs, oldest created first, with the default flagged. */
  async function listBaseCvs(): Promise<CvBaseItem[]> {
    return requestData<CvBaseItem[]>('/api/v1/me/cvs/bases');
  }

  /** The agent conversation for a BASE CV (no vacancy): the one already bound to it, or a new one.
   *  Costs nothing, so a free account can open the workspace; the paid gate guards each message. */
  async function startBaseSession(id: string): Promise<{ base_cv_id: string; session_id: string }> {
    return requestData<{ base_cv_id: string; session_id: string }>(
      `/api/v1/me/cvs/${encodeURIComponent(id)}/base-session`,
      jsonBody('POST', {}),
    );
  }

  /** Copy a base CV as "<title> (copy)"; returns the new record. 409 for a tailored copy. */
  async function duplicateCv(id: string): Promise<CvRecord> {
    return requestData<CvRecord>(`/api/v1/me/cvs/${encodeURIComponent(id)}/duplicate`, jsonBody('POST', {}));
  }

  /** Bind a roy agent session to a CV so its workspace can re-open that exact session. */
  async function setCvSession(id: string, sessionId: string): Promise<void> {
    await call(`/api/v1/me/cvs/${id}/session`, jsonBody('PUT', { session_id: sessionId }));
  }

  /** Fetch one CV with its full document. */
  async function getCv(id: string): Promise<CvRecord> {
    return requestData<CvRecord>(`/api/v1/me/cvs/${id}`);
  }

  /** The stored cover letter for the vacancy a tailored CV was written for. Never calls a
   *  model and consumes no allowance: `present: false` means the pair was never drafted. */
  async function getCoverLetter(id: string): Promise<CoverLetterView> {
    return requestData<CoverLetterView>(`/api/v1/me/cvs/${encodeURIComponent(id)}/cover-letter`);
  }

  /** What tailoring did to a tailored CV's ATS readiness, against the base CV it came from.
   *  Cookie-only and recomputed per request. 409 for a CV that is not a tailored copy; the
   *  response itself reports `available: false` when the comparison could not be made. */
  /** The history of what changed this CV, newest first. Each entry names who made it and
   *  which parts of the document it touched — the addresses the preview underlines. */
  async function listCvRevisions(id: string): Promise<RevisionView[]> {
    return requestData<RevisionView[]>(`/api/v1/me/cvs/${encodeURIComponent(id)}/revisions`);
  }

  /** Undo one entry. Edits made after it survive: only what that revision did is reversed.
   *  409 when its inverse no longer applies — the part of the CV it changed is gone. */
  async function undoCvRevision(cvId: string, revisionId: string): Promise<CvMeta> {
    const { cv } = await requestData<{ cv: CvMeta; revision: RevisionView }>(
      `/api/v1/me/cvs/${encodeURIComponent(cvId)}/revisions/${encodeURIComponent(revisionId)}/undo`,
      jsonBody('POST', {}),
    );
    return cv;
  }

  /** Undo every standing edit of one assistant run, newest first. */
  async function undoCvRevisionRun(cvId: string, batchId: string): Promise<CvMeta> {
    return requestData<CvMeta>(
      `/api/v1/me/cvs/${encodeURIComponent(cvId)}/revisions/batch/${encodeURIComponent(batchId)}/undo`,
      jsonBody('POST', {}),
    );
  }

  /** Rebuild this tailored CV (and the base) from the current seed — the experience bank
   *  first, the résumé's own extract only for what the bank doesn't track. Cookie-only.
   *  Same id and agent session; presentation preserved. 409 when the CV is not tailored or
   *  there is nothing to seed from. */
  async function reseedCv(id: string): Promise<CvRecord> {
    return requestData<CvRecord>(`/api/v1/me/cvs/${encodeURIComponent(id)}/reseed`, jsonBody('POST', {}));
  }

  /** Rebuild the base CV from the current seed. Cookie-only. Does not touch
   *  tailored copies. 409 when there is nothing to seed from. */
  async function reseedBaseCv(): Promise<CvRecord> {
    return requestData<CvRecord>('/api/v1/me/cvs/base/reseed', jsonBody('POST', {}));
  }

  async function getCvAtsDelta(id: string): Promise<CvAtsDelta> {
    return requestData<CvAtsDelta>(`/api/v1/me/cvs/${id}/ats-delta`);
  }

  /** How well a tailored CV matches its vacancy, scored on the current document. Cookie-only
   *  and recomputed per request. 409 for a CV bound to no vacancy; the response itself reports
   *  `available: false` when no score could be produced. */
  async function getCvJobMatch(id: string): Promise<CvJobMatch> {
    return requestData<CvJobMatch>(`/api/v1/me/cvs/${id}/job-match`);
  }

  /** Replace a CV's title, template, and document. */
  async function updateCv(id: string, input: UpdateCvInput): Promise<CvMeta> {
    return requestData<CvMeta>(`/api/v1/me/cvs/${id}`, jsonBody('PUT', input));
  }

  /** Turn link tracing on or off for one CV. 409 when the deployment has no visitor salt: there
   *  is then no honest way to count visitors, so the consent is refused rather than accepted and
   *  quietly under-recorded. Turning it OFF is never refused. */
  async function setCvTracerLinks(id: string, enabled: boolean): Promise<void> {
    await call(`/api/v1/me/cvs/${id}/tracer-links`, jsonBody('PUT', { enabled }));
  }

  /** What is known about each of a CV's traced links. Empty for a CV that was never traced. */
  async function listCvTracerLinks(id: string): Promise<CvTracerLink[]> {
    return requestData<CvTracerLink[]>(`/api/v1/me/cvs/${id}/tracer-links`);
  }

  /** Delete a CV. */
  async function deleteCv(id: string): Promise<void> {
    await call(`/api/v1/me/cvs/${id}`, { method: 'DELETE' });
  }

  /** The authenticated PDF URL for a CV (same-origin cookie rides along on download). */
  function cvPdfUrl(id: string): string {
    return `${baseUrl}/api/v1/me/cvs/${id}/pdf`;
  }

  /**
   * Bootstrap a tailoring session for a vacancy: seeds/creates the base CV, makes a
   * vacancy-bound tailored copy, and returns its id plus the cached analysis. Requires a
   * cached match analysis and a stored résumé (409 otherwise); beta-gated.
   */
  async function tailorCv(jobSlug: string, baseCvId?: string): Promise<TailorResult> {
    return requestData<TailorResult>(
      '/api/v1/me/cvs/tailor',
      jsonBody('POST', baseCvId ? { job_slug: jobSlug, base_cv_id: baseCvId } : { job_slug: jobSlug }),
    );
  }

  /**
   * Re-establish a tailoring session for an EXISTING tailored CV that has no bound agent session
   * (one created before session binding): mints a fresh CLI token and returns the CV + base ids
   * so the workspace can seed a new agent session against the same CV.
   */
  async function startTailorSession(id: string): Promise<TailorResult> {
    return requestData<TailorResult>(`/api/v1/me/cvs/${id}/tailor-session`, jsonBody('POST', {}));
  }

  /**
   * Turn a job slug, an external URL, or pasted JD text into a job slug the tailoring
   * workspace can open. Exactly one of `job_slug`/`url`/`text` must be set. A URL a
   * recognized ATS can read becomes a normal catalog job; anything else (a generic scrape
   * or plain text) becomes a private job — visible only via its own slug, never listed or
   * searchable. 422 when a URL cannot be read at all.
   */
  async function resolveJd(input: JdResolveInput): Promise<string> {
    const { job_slug } = await requestData<{ job_slug: string }>('/api/v1/me/jd/resolve', jsonBody('POST', input));
    return job_slug;
  }

  return {
    getPhoto,
    putPhoto,
    deletePhoto,
    listCvTemplates,
    listCvFonts,
    setCvTemplate,
    getCvAppearanceDefaults,
    setCvAppearanceDefaults,
    listCvs,
    listBaseCvs,
    startBaseSession,
    duplicateCv,
    setCvSession,
    getCv,
    getCoverLetter,
    listCvRevisions,
    undoCvRevision,
    undoCvRevisionRun,
    reseedCv,
    reseedBaseCv,
    getCvAtsDelta,
    getCvJobMatch,
    updateCv,
    setCvTracerLinks,
    listCvTracerLinks,
    deleteCv,
    cvPdfUrl,
    tailorCv,
    startTailorSession,
    resolveJd,
  };
}
