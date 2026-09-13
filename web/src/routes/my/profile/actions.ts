// Shared profile-section mutation callbacks. These touch only the existing app-wide
// singleton stores (profileStore, resumeStore) — never local component state — so
// they are plain functions rather than anything reactive, imported directly by
// whichever leaf page needs them, the same way those stores themselves are imported.

import { profileStore } from '$lib/profile.svelte';
import { syncProfileAlert } from '$lib/profileAlertSync';
import { resumeStore } from '$lib/resume.svelte';

// Fired after any Role/Skills/Location change, wherever it happens now (ProfileForm's
// batched Save during set-up, or a section's own per-field autosave).
export function handleSaved() {
  void syncProfileAlert();
}

export function handleCvUploaded() {
  resumeStore.noteUpload();
}

export function handleCvDeleted() {
  void resumeStore.refresh();
  // Education lives on profile.cv, sourced from resume_structured — clearing the file
  // server-side does not touch user_profiles, so this store has to re-fetch too or that
  // section stays stale. (Headline/summary/languages/certifications come from
  // resumeStore instead — see CvSummaryCard — so the refresh above already covers them.)
  void profileStore.refresh();
}
