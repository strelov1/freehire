// Shared profile-section mutation callbacks. These touch only the existing app-wide
// singleton stores (profileStore, resumeStore, savedSearches) — never local component
// state — so they are plain functions rather than anything reactive, imported directly
// by whichever leaf page needs them, the same way those stores themselves are imported.

import { filtersFromProfile, filtersToParams } from '$lib/filters';
import { profileStore } from '$lib/profile.svelte';
import { resumeStore } from '$lib/resume.svelte';
import { savedSearches } from '$lib/savedSearches.svelte';

// Keep the profile-derived saved search (the "notify me about jobs matching my
// profile" toggle, on the Search alerts page — ProfileAlertToggle) in step with a
// changed role/skills/location — otherwise it would keep alerting on the profile as it
// stood when first enabled. Best-effort: a failure here never blocks or rolls back the
// profile save itself, it just leaves the alert stale until the next successful save.
async function syncProfileAlert() {
  const p = profileStore.profile;
  if (!p) return;
  await savedSearches.ensureLoaded();
  const existing = savedSearches.items.find((s) => s.derived_from_profile);
  if (!existing) return;
  try {
    await savedSearches.update(existing.id, {
      query: filtersToParams(filtersFromProfile(p)).toString(),
    });
  } catch {
    // best-effort — see doc comment.
  }
}

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
