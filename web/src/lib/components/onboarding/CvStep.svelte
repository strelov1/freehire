<script lang="ts">
  // The CV step, with its two co-equal entry points: upload a PDF, or hand us a public
  // LinkedIn profile. Both fold into the same staged facets through the same dictionaries —
  // a profile is one more source of evidence about the candidate, not a different kind of
  // thing, so it must not merge by different rules.
  //
  // Everything here is step-local (the two in-flight states, their generation counters, the
  // notes) except the staged facets themselves, which belong to the wizard and come back
  // out through `onExtracted`.
  import { FileUp, LoaderCircle } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB } from '$lib/api';
  import { cvUploadReason, track } from '$lib/analytics';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { resumeStore } from '$lib/resume.svelte';
  import { MAX_SPECIALIZATIONS } from '$lib/profileLimits';
  import type { DerivedLocation } from '$lib/types';
  import { mergeFacets, type MergedFacets, type StagedFacets } from '$lib/onboardingImport';

  interface Props {
    /** What the wizard has staged so far — the merge folds into this rather than replacing
     *  it, so a manual pick made before an upload is not thrown away by it. */
    staged: StagedFacets;
    onExtracted: (merged: MergedFacets) => void;
    /** An address read off a LinkedIn profile. Handed on as a DERIVED location, never as a
     *  stated preference: it is something we worked out about the candidate, not something
     *  they told us. */
    onDerivedLocation: (location: DerivedLocation) => void;
    /** The LinkedIn URL the candidate typed here, so the confirm step's LinkedIn field is
     *  already filled with the address they just gave us. The CV route has no equivalent:
     *  the extract endpoint returns facets only, and the links a résumé carries reach the
     *  wizard through GET /me/resume instead. */
    onLinkedInUrl: (url: string) => void;
  }

  let { staged, onExtracted, onDerivedLocation, onLinkedInUrl }: Props = $props();

  let cvState = $state<'idle' | 'parsing' | 'error'>('idle');
  let cvError = $state<string | null>(null);
  let cvNote = $state<string | null>(null);
  let cvInput = $state<HTMLInputElement>();
  let cvGen = 0;

  // Defense in depth: a session expiring mid-visit can flip isAuthenticated() back to false
  // while this step is still on screen — extractResumeProfile would just 401. The wizard's
  // own guard sends the page away the next time it re-runs; this just avoids opening the
  // file picker in the meantime.
  function pickCv() {
    if (!isAuthenticated()) return;
    cvInput?.click();
  }

  // What an import says afterwards. Both entry points share it because they share the fold
  // (mergeFacets) — including the specialization cap, which is the one part of the result
  // the candidate cannot see for themselves: a role the cap left out simply is not on the
  // next step, and an import that quietly kept 10 of 13 reads as one that misread the CV.
  function importNote(merged: MergedFacets, source: string): string {
    if (!merged.resolved) return `Couldn’t read details from ${source} — pick below.`;
    if (merged.specializationsDropped > 0) {
      const n = merged.specializationsDropped;
      return `Filled in what we found — review on the next step. A profile holds ${MAX_SPECIALIZATIONS} specializations, so ${n} more we found ${n === 1 ? 'was' : 'were'} left out.`;
    }
    return 'Filled in what we found — review on the next step.';
  }

  async function onCvFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow re-picking the same file after an error
    if (!file) return;
    const gen = ++cvGen;
    cvState = 'parsing';
    cvError = null;
    cvNote = null;
    try {
      const cv = await api.extractResumeProfile(file);
      track('cv_upload', { ok: true, origin: 'onboarding_gate' });
      // Marks the CV present so a later visit does not redirect back here — does NOT
      // navigate away itself: the candidate stays to review the extracted fields on the
      // next steps.
      resumeStore.noteUpload();
      if (gen !== cvGen) return; // superseded by another pick or a page reset
      const merged = mergeFacets(staged, cv);
      onExtracted(merged);
      cvState = 'idle';
      cvNote = importNote(merged, 'that CV');
    } catch (err) {
      track('cv_upload', {
        ok: false,
        origin: 'onboarding_gate',
        reason: err instanceof ApiError ? cvUploadReason(err.message) : 'other',
      });
      if (gen !== cvGen) return;
      cvState = 'error';
      cvError = err instanceof ApiError ? err.message : 'Could not read the CV. Please try again.';
    }
  }

  let liUrl = $state('');
  let liState = $state<'idle' | 'loading' | 'error'>('idle');
  let liError = $state<string | null>(null);
  let liNote = $state<string | null>(null);
  let liGen = 0;

  async function importLinkedIn() {
    if (!isAuthenticated()) return;
    const url = liUrl.trim();
    if (!url || liState === 'loading') return;

    const gen = ++liGen;
    liState = 'loading';
    liError = null;
    liNote = null;
    try {
      const li = await api.importLinkedInProfile(url);
      track('linkedin_import', { ok: true, origin: 'onboarding_gate' });
      if (gen !== liGen) return; // superseded by another import or a page reset
      const merged = mergeFacets(staged, li);
      onExtracted(merged);
      if (li.location) onDerivedLocation(li.location);
      // The candidate just typed their own LinkedIn to import it. Asking again for a URL
      // they already gave us one step later reads as not having listened.
      onLinkedInUrl(url);
      liState = 'idle';
      liNote = importNote(merged, 'that profile');
    } catch (err) {
      track('linkedin_import', { ok: false, origin: 'onboarding_gate' });
      if (gen !== liGen) return;
      liState = 'error';
      liError = err instanceof ApiError ? err.message : 'Could not read that profile. Please try again.';
    }
  }
</script>

<h2 class="text-xl font-semibold tracking-tight">Upload your CV</h2>
<p class="mt-1 text-sm text-muted-foreground">We'll use it to fill in your role, skills, and level — you can always skip this.</p>

<input type="file" accept=".pdf,application/pdf" bind:this={cvInput} onchange={onCvFile} class="hidden" />
<button
  type="button"
  onclick={pickCv}
  disabled={cvState === 'parsing'}
  class="mt-4 flex w-full items-center justify-center gap-2 rounded-xl border border-dashed border-border bg-card px-4 py-3 text-sm font-medium transition-colors hover:border-brand hover:bg-accent disabled:opacity-60"
>
  {#if cvState === 'parsing'}
    <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Reading your CV…
  {:else}
    <FileUp class="size-4" aria-hidden="true" /> Upload CV
  {/if}
</button>
{#if cvState === 'error'}
  <p class="mt-2 text-xs text-destructive">{cvError}</p>
{:else if cvNote}
  <p class="mt-2 text-xs text-muted-foreground">{cvNote}</p>
{:else}
  <p class="mt-2 text-xs text-muted-foreground">PDF with selectable text, up to {RESUME_MAX_MB} MB.</p>
{/if}

<!-- The second entry point. Co-equal with the dropzone, not a fallback under it: a user
     with no PDF should not have to work out that the greyed-out half of the step is the one
     meant for them. -->
<div class="mt-5 flex items-center gap-3">
  <div class="h-px flex-1 bg-border"></div>
  <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">or</span>
  <div class="h-px flex-1 bg-border"></div>
</div>

<form class="mt-4 flex gap-2" onsubmit={(e) => { e.preventDefault(); void importLinkedIn(); }}>
  <input
    bind:value={liUrl}
    type="text"
    inputmode="url"
    autocomplete="url"
    placeholder="linkedin.com/in/your-name"
    aria-label="Your LinkedIn profile link"
    disabled={liState === 'loading'}
    class="min-w-0 flex-1 rounded-xl border border-input bg-card px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-ring disabled:opacity-60"
  />
  <button
    type="submit"
    disabled={liState === 'loading' || liUrl.trim() === ''}
    class="inline-flex shrink-0 items-center justify-center gap-2 rounded-xl border border-border bg-card px-4 py-2.5 text-sm font-medium transition-colors hover:border-brand hover:bg-accent disabled:opacity-60"
  >
    {#if liState === 'loading'}
      <LoaderCircle class="size-4 animate-spin" aria-hidden="true" /> Reading…
    {:else}
      Import
    {/if}
  </button>
</form>

{#if liState === 'error'}
  <p class="mt-2 text-xs text-destructive">{liError}</p>
{:else if liNote}
  <p class="mt-2 text-xs text-muted-foreground">{liNote}</p>
{/if}

<!-- Said before anyone tries it, not after it disappoints them. LinkedIn does not release
     work history to a reader who is not signed in, so this fills your role, skills, level
     and location and nothing else. -->
<p class="mt-2 text-xs text-muted-foreground">
  LinkedIn only shares your headline and location publicly — not your work history.
  To bring that in, open your profile on LinkedIn, choose <span class="font-medium text-foreground">More → Save to PDF</span>, and upload the file above.
</p>
