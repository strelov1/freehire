<script lang="ts">
  // The CV step: upload a PDF and let its facets fold into the wizard's staged set.
  //
  // There was a second entry point here — paste a public LinkedIn profile link and read the
  // same facets off the page. It is gone. LinkedIn answers a datacentre address with its
  // block status rather than a profile, so in production the import failed for ~87% of the
  // people who tried it (103 of 119 attempts in the week to 2026-09-10) and the step's first
  // impression was an error. What it could ever have yielded was the headline and the city;
  // the "Save to PDF" hint below is the same profile, read the way LinkedIn does release it.
  //
  // Everything here is step-local (the in-flight state, its generation counter, the notes)
  // except the staged facets themselves, which belong to the wizard and come back out
  // through `onExtracted`.
  import { FileUp, LoaderCircle } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB } from '$lib/api';
  import { cvUploadReason, track } from '$lib/analytics';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { resumeStore } from '$lib/resume.svelte';
  import { MAX_SPECIALIZATIONS } from '$lib/profileLimits';
  import { mergeFacets, type MergedFacets, type StagedFacets } from '$lib/onboardingImport';

  interface Props {
    /** What the wizard has staged so far — the merge folds into this rather than replacing
     *  it, so a manual pick made before an upload is not thrown away by it. */
    staged: StagedFacets;
    onExtracted: (merged: MergedFacets) => void;
    /** A CV has just been stored, so its background parse has just started. Fired from here
     *  rather than from the wizard's Continue because the two are seconds apart, and those
     *  are seconds of a wait the candidate would otherwise spend on the next step for
     *  nothing. */
    onCvUploaded: () => void;
    /** Leave this step for the next one, saving it the way Continue would. */
    onAdvance: () => void;
  }

  let { staged, onExtracted, onCvUploaded, onAdvance }: Props = $props();

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

  // What an upload says afterwards — including the specialization cap, which is the one part
  // of the result the candidate cannot see for themselves: a role the cap left out simply is
  // not on the next step, and an import that quietly kept 10 of 13 reads as one that misread
  // the CV.
  function importNote(merged: MergedFacets): string {
    if (!merged.resolved) return 'Couldn’t read details from that CV — pick below.';
    if (merged.specializationsDropped > 0) {
      const n = merged.specializationsDropped;
      return `Filled in what we found — review on the next step. A profile holds ${MAX_SPECIALIZATIONS} specializations, so ${n} more we found ${n === 1 ? 'was' : 'were'} left out.`;
    }
    return 'Filled in what we found — review on the next step.';
  }

  // Whether an upload should carry the candidate onward by itself. It should when the note it
  // would leave behind says nothing they need to act on: "review on the next step" is a
  // promise best kept by going there, and staying put after a successful upload reads as the
  // upload not having registered.
  //
  // The two notes that DO say something — an upload that recognised nothing, and one whose
  // roles the specialization cap trimmed — keep the candidate here to read them. Both are
  // about this step, and neither survives the screen it was written for.
  function shouldAdvanceAfter(merged: MergedFacets): boolean {
    return merged.resolved && merged.specializationsDropped === 0;
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
      // The CV is stored by now, so its structured parse is already running server-side.
      // Telling the wizard here rather than on Continue is what gives that parse the seconds
      // it needs before the links step asks for its result.
      onCvUploaded();
      const merged = mergeFacets(staged, cv);
      onExtracted(merged);
      cvState = 'idle';
      cvNote = importNote(merged);
      if (shouldAdvanceAfter(merged)) onAdvance();
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

<!-- The route for a candidate who has no PDF to hand. It is a hint rather than a second
     control on purpose: LinkedIn releases a signed-out reader the headline and the city and
     nothing else, so a button promising to read a profile link would promise more than the
     page behind it can give. -->
<p class="mt-4 text-xs text-muted-foreground">
  No CV file? Open your profile on LinkedIn, choose <span class="font-medium text-foreground">More → Save to PDF</span>, and upload that.
</p>
