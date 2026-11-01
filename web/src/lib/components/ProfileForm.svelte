<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { messages } from './ProfileForm.messages';
  import { ArrowUp, Check, Trash2 } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB } from '#lib/api.js';
  import { cvUploadReason, track } from '#lib/analytics.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { capSpecializations, MAX_SPECIALIZATIONS } from '#lib/profileLimits.js';
  import type { UserProfile } from '#lib/types.js';
  import { withSkills } from '#lib/profileSkills.js';
  import { Button, ConfirmDialog } from '#lib/ui/index.js';
  import { announce } from '#lib/announce.svelte.js';
  import HeadshotField from './HeadshotField.svelte';

  // Photo and CV management. Roles, skills and location autosave in their own
  // sections; a CV import merges its extracted skills and roles into the same profile.
  let {
    profile,
    hasCv,
    uploadedAt = null,
    onSaved,
    onCvUploaded,
    onCvDeleted,
  }: {
    profile: UserProfile;
    hasCv: boolean;
    /** When the stored CV was uploaded (ISO). Shown in the uploaded-state box instead of
     *  the bare "CV on file" a candidate had no way to act on — a date at least answers
     *  "is this current". */
    uploadedAt?: string | null;
    onSaved?: () => void;
    onCvUploaded?: () => void;
    /** Fired after the stored CV is deleted, so the parent can refresh `hasCv`/the
     *  résumé meta it reads elsewhere on the page. */
    onCvDeleted?: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));

  const uploadedAtLabel = $derived(
    uploadedAt
      ? format(s.uploaded, { date: new Date(uploadedAt).toLocaleDateString(locale(), { year: 'numeric', month: 'short', day: 'numeric' }) })
      : s.onFile,
  );

  let resumeBusy = $state(false);
  let resumeError = $state<string | null>(null);
  let resumeNote = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let dragActive = $state(false);


  // Each message below appears where the action was, which a screen reader never visits on
  // its own — so each is also spoken the moment it changes. Errors interrupt; notes wait.
  $effect(() => {
    if (resumeError) announce(resumeError, 'assertive');
  });
  $effect(() => {
    if (resumeNote) announce(resumeNote);
  });

  // Merge the extraction into the current store copy, preserving manual additions.
  async function analyzeResume(file: File) {
    resumeBusy = true;
    resumeError = null;
    resumeNote = null;
    try {
      const cv = await api.extractResumeProfile(file);
      track('cv_upload', { ok: true, origin: 'profile' });
      onCvUploaded?.();

      const current = profileStore.profile ?? profile;
      const { kept: nextSpecializations, dropped: cappedSpecs } =
        capSpecializations(current.specializations, cv.categories);
      const addedSpecs = nextSpecializations.length - current.specializations.length;
      const addedSkills = withSkills(current, cv.skills).skills.length - current.skills.length;
      if (addedSkills > 0 || addedSpecs > 0) {
        await profileStore.mergeResumeExtraction(cv.skills, nextSpecializations);
        onSaved?.();
      }

      if (addedSkills > 0 || addedSpecs > 0) {
        resumeNote = format(s.added, { skills: String(addedSkills), roles: String(addedSpecs) });
        if (addedSkills > 0) resumeNote += ' ' + s.seeSkills;
      } else if (cappedSpecs > 0)
        resumeNote = format(s.limitReached, { max: String(MAX_SPECIALIZATIONS) });
      else if (cv.skills.length === 0 && cv.categories.length === 0) resumeNote = s.noSkills;
      else resumeNote = s.alreadyListed;
    } catch (err) {
      resumeError = err instanceof ApiError ? err.message : s.cvReadError;
      // The reason goes out as a bounded code, never the message itself: the copy is
      // written for the user and rewording it must not split the metric in two.
      track('cv_upload', {
        ok: false,
        origin: 'profile',
        reason: err instanceof ApiError ? cvUploadReason(err.message) : 'other',
      });
    } finally {
      resumeBusy = false;
    }
  }

  let confirmDeleteCvOpen = $state(false);
  let deletingCv = $state(false);

  async function deleteCv() {
    deletingCv = true;
    resumeError = null;
    resumeNote = null;
    try {
      await api.deleteResume();
      onCvDeleted?.();
    } catch (err) {
      resumeError = err instanceof ApiError ? err.message : s.cvDeleteError;
    } finally {
      deletingCv = false;
    }
  }

  function onResumeFile(e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    const file = target.files?.[0];
    target.value = ''; // clear so re-selecting the same file fires change again
    if (file) void analyzeResume(file);
  }

  function isPdf(file: File): boolean {
    return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
  }

  // The file input's `accept` filters the picker, but a drop bypasses it — so the drop
  // path validates the type itself before touching the extractor.
  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    if (resumeBusy) return; // the box is a plain div, not a disabled button — enforce it here
    const file = e.dataTransfer?.files?.[0];
    if (!file) return;
    if (!isPdf(file)) {
      resumeError = s.dropPdfError;
      return;
    }
    void analyzeResume(file);
  }

</script>

<div class="flex flex-col gap-6">
  <!-- Your photo: one headshot per member, printed by the CV templates that show one.
       Renders nothing when object storage is unconfigured. -->
  <HeadshotField />

  <!-- Your CV: the same dashed drop-zone box in both states — empty (drop/choose a PDF)
       or with one on file (uploaded indicator + Replace/Delete), so the box the candidate
       first uploaded to is still where they manage it, not a bare row that replaces it.
       A div, not a button, once a CV exists: Replace and Delete are their own buttons,
       which can't nest inside one. Drag-to-replace still works on the box either way.
       Uploading merges extracted roles and skills straight into the profile. -->
  <!-- `account-cv` is the anchor the account-setup checklist's CV step links to (see
       accountCompleteness.ts) — this box is the only place under /my/ that takes a base
       résumé, /my/cvs being the per-vacancy builder. `scroll-mt-20` clears the sticky
       `h-14` TopBar. -->
  <div id="account-cv" class="flex scroll-mt-20 flex-col gap-1.5">
    <span class="text-sm font-medium">{s.cv}</span>
    <input
      type="file"
      accept="application/pdf,.pdf"
      class="hidden"
      bind:this={fileInput}
      onchange={onResumeFile}
    />
    <!-- A drop-target enhancement over controls (the buttons below) that are already
         fully keyboard-accessible on their own. -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      ondragover={(e) => {
        e.preventDefault();
        if (!resumeBusy) dragActive = true;
      }}
      ondragleave={(e) => {
        e.preventDefault();
        dragActive = false;
      }}
      ondrop={onDrop}
      class="flex items-center justify-center gap-3 rounded-xl border-2 border-dashed px-6 py-8 text-center transition-colors {dragActive
        ? 'border-brand bg-brand/5'
        : 'border-border'}"
    >
      {#if hasCv}
        <span class="flex size-11 items-center justify-center rounded-full bg-brand text-brand-foreground">
          <Check class="size-5" />
        </span>
        <span class="flex flex-col items-start gap-1.5 text-left">
          <span class="text-sm font-semibold">{resumeBusy ? s.analyzing : uploadedAtLabel}</span>
          <span class="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={resumeBusy} onclick={() => fileInput?.click()}>
              {s.replace}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={resumeBusy || deletingCv}
              onclick={() => (confirmDeleteCvOpen = true)}
              class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            >
              <Trash2 class="size-4" />
              {s.delete}
            </Button>
          </span>
        </span>
      {:else}
        <button
          type="button"
          onclick={() => fileInput?.click()}
          disabled={resumeBusy}
          class="flex items-center gap-3 disabled:opacity-70"
        >
          <span class="flex size-11 items-center justify-center rounded-full bg-brand text-brand-foreground">
            <ArrowUp class="size-5" />
          </span>
          <span class="flex flex-col gap-0.5 text-left">
            <span class="text-sm font-semibold">{resumeBusy ? s.analyzing : s.dropPdf}</span>
            {#if !resumeBusy}
              <span class="text-xs text-muted-foreground">
                <span class="text-primary underline">{s.chooseFile}</span>
              </span>
            {/if}
          </span>
        </button>
      {/if}
    </div>
    <span class="text-xs text-muted-foreground">
      {format(s.cvHintEditing, { max: String(RESUME_MAX_MB) })}
    </span>
    {#if resumeError}
      <p class="text-sm text-destructive">{resumeError}</p>
    {:else if resumeNote}
      <p class="text-xs text-muted-foreground">{resumeNote}</p>
    {/if}
  </div>


</div>

<ConfirmDialog
  bind:open={confirmDeleteCvOpen}
  title={s.deleteCvTitle}
  description={s.deleteCvDescription}
  confirmLabel={s.delete}
  variant="destructive"
  onConfirm={deleteCv}
/>
