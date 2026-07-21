// CV-builder view types and pure editor helpers. The server owns the Document wire
// shape (generated into contracts.ts); this module adds the list/detail response shapes
// and the blank-entry factories the section form uses to add rows. Kept Svelte-free and
// unit-testable.

import type {
  Document,
  ExperienceItem,
  EducationItem,
  SkillGroup,
  Language,
  Project,
  Certification,
  Analysis,
} from './generated/contracts';

/** CV metadata (list rows and mutation responses). */
export interface CvMeta {
  id: number;
  title: string;
  template_id: string;
  created_at: string;
  updated_at: string;
}

/** A CV with its full editable document. `agent_session_id` is the roy session bound to a
 *  tailored CV (empty when none) — the workspace resumes it. */
export interface CvRecord extends CvMeta {
  agent_session_id: string;
  document: Document;
}

/** A tailored CV in the /my/cvs re-open list: metadata plus the vacancy (slug, title, company)
 *  and the bound agent session, so a row renders as a company card linking to its workspace. */
export interface CvTailoredItem extends CvMeta {
  job_slug: string;
  job_title: string;
  job_company: string;
  agent_session_id: string;
}

/**
 * Result of bootstrapping a tailoring session: the ids of the new vacancy-bound CV and the
 * base it was copied from, the cached fit analysis, and the short-lived token the agent's CLI
 * authenticates with. `cli_token` is handed to the agent session; the browser only needs the
 * ids (to open the tailored CV) and the analysis (to show context).
 */
export interface TailorResult {
  tailor_cv_id: number;
  base_cv_id: number;
  analysis: Analysis | null;
  cli_token: string;
}

/** Request body for replacing a CV. */
export interface UpdateCvInput {
  title: string;
  template_id: string;
  document: Document;
}

export const DEFAULT_TEMPLATE_ID = 'classic-ats';

/** A CV template the user can pick in the gallery. `ats_safe` is false for richer layouts
 *  (e.g. the sidebar) that may not parse cleanly in some ATS. Mirrors cv.TemplateInfo. */
export interface CvTemplate {
  id: string;
  label: string;
  style: string;
  ats_safe: boolean;
}

/** A fresh, fully-populated (but empty) document so the form can bind every section
 *  without null-guards. The server still sanitizes on save, dropping the empties. */
export function emptyDocument(): Document {
  return {
    header: { full_name: '', email: '', phone: '', location: '', links: [] },
    summary: '',
    experience: [],
    education: [],
    skills: [],
    languages: [],
    projects: [],
    certifications: [],
  };
}

/** Merge a possibly-partial document (from the API, where empty sections are omitted)
 *  into a full shape the form can bind to. */
export function toEditable(doc: Document): Document {
  const base = emptyDocument();
  return {
    header: { ...base.header, ...doc.header, links: doc.header?.links ?? [] },
    summary: doc.summary ?? '',
    experience: (doc.experience ?? []).map((e) => ({ ...e, bullets: e.bullets ?? [], stack: e.stack ?? [] })),
    education: doc.education ?? [],
    skills: (doc.skills ?? []).map((s) => ({ ...s, items: s.items ?? [] })),
    languages: doc.languages ?? [],
    projects: (doc.projects ?? []).map((p) => ({ ...p, bullets: p.bullets ?? [] })),
    certifications: doc.certifications ?? [],
  };
}

export const blankExperience = (): ExperienceItem => ({
  role: '',
  company: '',
  location: '',
  start: '',
  end: '',
  current: false,
  summary: '',
  bullets: [''],
  stack: [],
});

export const blankEducation = (): EducationItem => ({
  institution: '',
  degree: '',
  field: '',
  start: '',
  end: '',
});

export const blankSkillGroup = (): SkillGroup => ({ group: '', items: [] });

export const blankLanguage = (): Language => ({ name: '', level: '' });

export const blankProject = (): Project => ({ name: '', link: '', bullets: [''] });

export const blankCertification = (): Certification => ({ name: '', issuer: '', year: '' });

/** The title to show for a CV whose title is blank. */
export function cvTitle(title: string): string {
  return title.trim() || 'Untitled CV';
}
