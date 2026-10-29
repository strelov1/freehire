// Shared profile-section mutation callbacks. These touch only the existing app-wide
// singleton stores (profileStore, resumeStore, experienceBankStore) — never local
// component state — so they are plain functions rather than anything reactive, imported
// directly by whichever leaf page needs them, the same way those stores themselves are
// imported.

import { experienceBankStore } from '#lib/experienceBank.svelte.js';
import { profileStore } from '#lib/profile.svelte.js';
import { syncProfileAlert } from '#lib/profileAlertSync.js';
import { resumeStore } from '#lib/resume.svelte.js';

// Fired after any Role/Skills/Location change, wherever it happens now (ProfileForm's
// batched Save during set-up, or a section's own per-field autosave).
export function handleSaved() {
  void syncProfileAlert();
}

// The upload answers before its background parse has run, and the summary, education and
// experience that parse writes land seconds later — read by the cards on these pages and
// by the tabs' completeness dots. So follow the parse, then re-read what it fed.
export async function handleCvUploaded() {
  resumeStore.noteUpload();
  if (await resumeStore.waitForParse()) {
    void profileStore.refresh();
    void experienceBankStore.refresh();
  }
}

export function handleCvDeleted() {
  void resumeStore.refresh();
  // Education lives on profile.cv, sourced from resume_structured — clearing the file
  // server-side does not touch user_profiles, so this store has to re-fetch too or that
  // section stays stale. (Headline/summary/languages/certifications come from
  // resumeStore instead — see CvSummaryCard — so the refresh above already covers them.)
  void profileStore.refresh();
}
