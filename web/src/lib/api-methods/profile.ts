import type { Http } from '../api-core';
import type { components } from '../generated/api.js';
import type {
  ATSResponse,
  CandidateContacts,
  ExperienceAtom,
  ExperienceBank,
  ExperienceEmployment,
  FacetCounts,
  LocationPreferences,
  ResumeMeta,
  ResumeProfile,
  TalentNetworkSetting,
  TalentNetworkVisibility,
  UserProfile,
  Verdict,
} from '../types';
// `Responses` is the onboarding survey's record. Aliased on the way in because the
// generated contracts are one flat namespace and the name says nothing on its own there —
// see cmd/gen-contracts for why it is not called `Answers` like its Go siblings.
import type { Answers, CatalogueMember, Responses as SurveyAnswers } from '#lib/generated/contracts.js';
import { ApiError, jsonBody, RESUME_MAX_BYTES, RESUME_MAX_MB, resumeInit, toSlice } from '../api-core';
import type { Page, RoastResponse, Slice } from '../api-types';

export function profileApi({ call, request, requestData }: Http) {
  // The experience bank: what the product has recorded about what the user has done.

  /** The caller's whole experience bank, grouped by role. */
  async function getExperience(): Promise<ExperienceBank> {
    return requestData<ExperienceBank>('/api/v1/me/experience');
  }

  /** Rewrite one achievement. Editing it re-stamps it as the owner's own statement, which
   *  is how something the assistant inferred becomes usable on a CV. */
  async function updateExperienceAtom(id: string, atom: Partial<ExperienceAtom>): Promise<ExperienceAtom> {
    return requestData<ExperienceAtom>(`/api/v1/me/experience/atoms/${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(atom),
    });
  }

  /** Fold two achievements into one richer keep. Cookie-only — merge deletes the other. */
  async function mergeExperienceAtoms(ids: [string, string]): Promise<ExperienceAtom> {
    return requestData<ExperienceAtom>('/api/v1/me/experience/atoms/merge', jsonBody('POST', { ids }));
  }

  /** Remove one achievement. This is the only path that takes evidence out of the bank. */
  async function deleteExperienceAtom(id: string): Promise<void> {
    await call(`/api/v1/me/experience/atoms/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  /** Remove a role, and with it the achievements that were evidence of it. */
  async function deleteExperienceEmployment(id: string): Promise<void> {
    await call(`/api/v1/me/experience/employments/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  /** Create a job or project employment in the bank. */
  async function createExperienceEmployment(
    body: Partial<ExperienceEmployment> & { kind: 'job' | 'project' },
  ): Promise<ExperienceEmployment> {
    return requestData<ExperienceEmployment>('/api/v1/me/experience/employments', jsonBody('POST', body));
  }

  /** Update employment metadata (name/company, link, dates). Cookie-only. */
  async function updateExperienceEmployment(
    id: string,
    body: Partial<ExperienceEmployment>,
  ): Promise<ExperienceEmployment> {
    return requestData<ExperienceEmployment>(
      `/api/v1/me/experience/employments/${encodeURIComponent(id)}`,
      jsonBody('PUT', body),
    );
  }

  // The single per-user profile: a specialization + skills set (cookie-only on the server).

  /** The current user's profile, or null when they have not saved one yet. */
  async function getProfile(): Promise<UserProfile | null> {
    return requestData<UserProfile | null>('/api/v1/me/profile');
  }

  /** Create-or-replace the user's profile from a non-empty set of specializations (job
   *  categories), a non-empty set of skills, an optional set of seniorities (desired
   *  levels; may be empty), an optional set of excluded skills/sources/companies (may be
   *  empty), and an optional location-preferences block (null clears it). A bad
   *  specialization, empty skills, an unknown seniority, or an out-of-vocabulary location
   *  value is a 400. */
  async function saveProfile(
    specializations: string[],
    skills: string[],
    seniorities: string[],
    excludedSkills: string[],
    excludedSources: string[],
    excludedCompanies: string[],
    location: LocationPreferences | null,
    jobLanguages: string[] = [],
  ): Promise<UserProfile> {
    return requestData<UserProfile>(
      '/api/v1/me/profile',
      jsonBody('PUT', {
        specializations,
        skills,
        seniorities,
        excluded_skills: excludedSkills,
        excluded_sources: excludedSources,
        excluded_companies: excludedCompanies,
        location_preferences: location,
        job_languages: jobLanguages,
      }),
    );
  }

  // The candidate's own screening answers — the handful of questions that repeat across
  // ATS application forms (visa, salary, notice period, relocation, …) and no CV states.
  // Distinct from the profile above: a different lifecycle, see
  // internal/screeninganswers/AGENTS.md.

  /** The current user's screening answers, or null when they have stated none yet. */
  async function getScreeningAnswers(): Promise<Answers | null> {
    return requestData<Answers | null>('/api/v1/me/screening-answers');
  }

  /** Partially update the user's screening answers: a field the patch omits keeps
   *  whatever was already stored, and a field named in `clear` is withdrawn. An
   *  unrecognized country code, a malformed currency, an out-of-vocabulary period, a
   *  non-positive salary, a negative notice period, or a field both stated and cleared is
   *  a 400 naming it. */
  async function updateScreeningAnswers(patch: Partial<Answers> & { clear?: (keyof Answers)[] }): Promise<Answers> {
    return requestData<Answers>('/api/v1/me/screening-answers', jsonBody('PUT', patch));
  }

  // The onboarding survey — what the candidate told us about their SEARCH (what is in its
  // way). Distinct from the screening answers above, which are what an employer sees; and
  // from the profile, which is a search filter.

  /** The current user's survey answers. Never null and never a 404: an account that has
   *  answered nothing reads an object with every field absent. */
  async function getSurvey(): Promise<SurveyAnswers> {
    return requestData<SurveyAnswers>('/api/v1/me/survey');
  }

  /** Partially update the user's survey answers: a field the patch omits keeps whatever was
   *  already stored, and there is no way to clear one back to unstated. A value outside its
   *  vocabulary, a note alongside a challenge other than `other`, a malformed currency, or a
   *  non-positive income is a 400 naming the offending field. */
  async function updateSurvey(patch: Partial<SurveyAnswers>): Promise<SurveyAnswers> {
    return requestData<SurveyAnswers>('/api/v1/me/survey', jsonBody('PUT', patch));
  }

  /** Record that this account has been through the onboarding flow, so the layout stops
   *  routing it there. Idempotent — a second call keeps the original timestamp. */
  async function completeOnboarding(): Promise<void> {
    await requestData<{ onboarding_complete: boolean }>('/api/v1/me/onboarding/complete', jsonBody('POST', {}));
  }

  // Talent Network: the caller's own membership toggle, and the public catalogue it puts
  // them in (internal/api/handler/me_talent_network.go and talent_catalog.go).

  /** The caller's current Talent Network visibility and public id. A user who has never
   *  touched the setting reads "off". */
  async function getTalentNetwork(): Promise<TalentNetworkSetting> {
    return requestData<TalentNetworkSetting>('/api/v1/me/talent-network');
  }

  /** Update the caller's own Talent Network visibility. Returns the echoed setting rather
   *  than assuming success, same as saveProfile. */
  async function setTalentNetworkVisibility(visibility: TalentNetworkVisibility): Promise<TalentNetworkSetting> {
    return requestData<TalentNetworkSetting>(
      '/api/v1/me/talent-network',
      jsonBody('PUT', { visibility } satisfies components['schemas']['TalentNetworkVisibilityUpdate']),
    );
  }

  /** One filtered page of the public Talent Network catalogue. Unauthenticated.
   *
   *  `search` is the already-serialised query string (writeTalentQuery's output), passed
   *  through verbatim rather than rebuilt here: the URL is the single source of truth for
   *  what is filtered, and a second serialiser is a second answer to what `?skills=go`
   *  means. */
  async function listTalent(search: string): Promise<Slice<CatalogueMember>> {
    const page = await request<Page<CatalogueMember>>(`/api/v1/talent${search ? `?${search}` : ''}`);
    return toSlice(page, page.meta.offset);
  }

  /** The catalogue's facet distribution under a filter: how many members sit behind each
   *  value of each facet, so a filter pane can show the number beside every option.
   *
   *  A count arrives NEGATIVE when the endpoint holds it back below its small-count floor;
   *  `reportedCount` in facets.ts is what turns that into "no number". */
  async function talentFacets(search: string): Promise<FacetCounts> {
    return requestData<FacetCounts>(`/api/v1/talent/facets${search ? `?${search}` : ''}`);
  }

  /** One member's public card, by their minted catalogue handle. A member who has left,
   *  a handle nobody holds, and a string that could not be a handle all answer the same
   *  404 — the caller must not try to tell them apart. */
  async function getTalentCard(handle: string): Promise<CatalogueMember> {
    return requestData<CatalogueMember>(`/api/v1/talent/${encodeURIComponent(handle)}`);
  }

  /** Permanently erase the signed-in account and everything it owns. Irreversible:
   *  there is no restore path on either side. `email` is the confirmation — it must be
   *  the caller's own address, or the server answers 400. A 503 means nothing was
   *  deleted (stored files were unreachable) and the request can be retried. */
  async function deleteAccount(email: string): Promise<void> {
    await call('/api/v1/me', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
    });
  }

  /** Derive a résumé's profile — a PDF `File` (sent as multipart) or pasted text (sent
   *  as JSON) — via the deterministic dictionaries: canonical skills plus the
   *  specializations and seniority resolved, ready to pre-fill a profile or the onboarding
   *  wizard. */
  async function extractResumeProfile(input: File | string): Promise<ResumeProfile> {
    if (input instanceof File && input.size > RESUME_MAX_BYTES) {
      throw new ApiError(
        413,
        `This PDF is larger than ${RESUME_MAX_MB} MB. Compress it or export a lighter PDF and try again.`,
      );
    }
    return requestData<ResumeProfile>('/api/v1/me/resume/extract', resumeInit('POST', input));
  }

  /** Store (or replace) the caller's CV, which also starts its free background parse. Fails
   *  loudly where `extractResumeProfile` would not: 501 means storage is unconfigured and
   *  nothing was stored. Cookie-only. */
  async function putResume(file: File): Promise<ResumeMeta> {
    if (file.size > RESUME_MAX_BYTES) {
      throw new ApiError(
        413,
        `This PDF is larger than ${RESUME_MAX_MB} MB. Compress it or export a lighter PDF and try again.`,
      );
    }
    return requestData<ResumeMeta>('/api/v1/me/resume', resumeInit('PUT', file));
  }

  /** The caller's résumé status, including the read-only structured résumé parsed from
   *  the CV (null when none is current). Session-scoped; always 200 (an absent/disabled
   *  résumé is a normal state the profile renders). */
  async function getResume(): Promise<ResumeMeta> {
    return requestData<ResumeMeta>('/api/v1/me/resume');
  }

  /** Delete the stored CV file and its structured extract. Owned contacts survive. */
  async function deleteResume(): Promise<void> {
    await call('/api/v1/me/resume', { method: 'DELETE' });
  }

  /** Re-derive the structured résumé from the already-stored upload, without re-submitting
   *  the file — for a candidate whose `parse_status` came back `failed`. Fires the same
   *  background derivation an upload triggers; the caller re-polls `getResume` for the
   *  outcome. */
  async function retryResumeExtract(): Promise<void> {
    await call('/api/v1/me/resume/retry-extract', { method: 'POST' });
  }

  /** Replace candidate-owned contacts without re-uploading a CV. */
  async function putResumeContacts(contacts: CandidateContacts): Promise<CandidateContacts> {
    return requestData<CandidateContacts>('/api/v1/me/resume/contacts', jsonBody('PUT', contacts));
  }

  /** The market-coverage verdict for the caller's profile: how many open vacancies the
   *  profile's skills reach for the selected role, and which missing skill unlocks the
   *  most new ones. `params` carries the same facet filters as job search, so the caller
   *  can recompute for an ad-hoc role; absent a `category` the server defaults to the
   *  profile's specializations. Session-scoped (404 when no profile). */
  async function getProfileVerdict(params?: URLSearchParams): Promise<Verdict> {
    const qs = params?.toString();
    return requestData<Verdict>(`/api/v1/me/profile/verdict${qs ? `?${qs}` : ''}`);
  }

  /** The CV ATS-readiness report for the caller's profile: structural checks over the
   *  stored CV plus a keyword-match against the selected role's top skills. `params`
   *  carries the same facet filters as the verdict. `has_cv` is false (report null)
   *  when no CV is stored — the page then prompts an upload. Session-scoped. */
  async function getATSReport(params?: URLSearchParams): Promise<ATSResponse> {
    const qs = params?.toString();
    return requestData<ATSResponse>(`/api/v1/me/profile/ats-report${qs ? `?${qs}` : ''}`);
  }

  /** Run the optional LLM qualitative review over the caller's stored CV; returns the
   *  ATS report with content-quality + findings folded in (cached server-side). With no
   *  LLM configured this is just the deterministic report. */
  async function runATSReview(params?: URLSearchParams): Promise<ATSResponse> {
    const qs = params?.toString();
    return requestData<ATSResponse>(`/api/v1/me/profile/ats-report${qs ? `?${qs}` : ''}`, { method: 'POST' });
  }

  /** Score an uploaded CV for an ANONYMOUS caller — no session, no stored bytes, no model
   *  call (see RoastCV on the backend). Same two input shapes as extractResumeProfile (a
   *  PDF `File` sent as multipart, or pasted text sent as JSON) through the same size
   *  check and the same `resumeInit` builder. `category` overrides the role the market
   *  reading is measured against; omitted, the server infers it from the CV's own
   *  headline. This route is public. */
  async function roastCv(input: File | string, category?: string): Promise<RoastResponse> {
    if (input instanceof File && input.size > RESUME_MAX_BYTES) {
      throw new ApiError(
        413,
        `This PDF is larger than ${RESUME_MAX_MB} MB. Compress it or export a lighter PDF and try again.`,
      );
    }
    const qs = category ? `?category=${encodeURIComponent(category)}` : '';
    return requestData<RoastResponse>(`/api/v1/cv/roast${qs}`, resumeInit('POST', input));
  }

  return {
    getExperience,
    updateExperienceAtom,
    mergeExperienceAtoms,
    deleteExperienceAtom,
    deleteExperienceEmployment,
    createExperienceEmployment,
    updateExperienceEmployment,
    getProfile,
    saveProfile,
    getScreeningAnswers,
    updateScreeningAnswers,
    getSurvey,
    updateSurvey,
    completeOnboarding,
    getTalentNetwork,
    setTalentNetworkVisibility,
    listTalent,
    talentFacets,
    getTalentCard,
    deleteAccount,
    extractResumeProfile,
    putResume,
    getResume,
    deleteResume,
    retryResumeExtract,
    putResumeContacts,
    getProfileVerdict,
    getATSReport,
    runATSReview,
    roastCv,
  };
}
