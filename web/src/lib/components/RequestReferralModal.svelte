<script lang="ts">
  import { onMount } from 'svelte';
  import { Check, FileUp, Paperclip, Sparkles } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB } from '#lib/api.js';
  import type { ReferralCvOptions, ReferralRequestInput } from '#lib/types.js';
  import { Button, Dialog, FormField } from '#lib/ui/index.js';
  import { isLinkedInUrl } from '#lib/utils.js';

  // The parent owns open/close; this component owns the request form. jobSlug is the
  // vacancy the request is made from, recorded with it.
  //
  // The CV it shares is one of two, never a pick from every CV the seeker owns: a CV
  // tailored for that vacancy — or, from the company page, for one of the company's
  // vacancies — attached by itself when there is one, or the CV file uploaded to the
  // profile. With neither it offers both ways in: upload a file, or (from a vacancy) tailor
  // one through the page's own flow.
  let {
    companySlug,
    companyName,
    jobSlug,
    onClose,
    onSent,
    onTailor,
  }: {
    companySlug: string;
    companyName: string;
    jobSlug?: string;
    onClose: () => void;
    onSent?: () => void;
    onTailor?: () => void;
  } = $props();

  // The parent mounts this component to open it and unmounts on onClose, so the
  // dialog is open for its whole life. Dialog owns the closing — Escape, the
  // backdrop and its own button all land on `open`, and this reports it upward
  // rather than each affordance calling onClose itself.
  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  let options = $state.raw<ReferralCvOptions | null>(null);
  // Which CV goes out; null until the seeker has one to share.
  let cvKind = $state<'original' | 'built' | null>(null);
  let cvId = $state<string | null>(null);
  let uploading = $state(false);
  let uploadError = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let linkedinUrl = $state('');
  let contactTelegram = $state('');
  let contactEmail = $state('');
  let note = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);
  let done = $state(false);

  const linkedinValid = $derived(isLinkedInUrl(linkedinUrl));
  const canSubmit = $derived(
    linkedinValid &&
      (contactTelegram.trim() !== '' || contactEmail.trim() !== '') &&
      (cvKind === 'original' || (cvKind === 'built' && cvId !== null)),
  );

  onMount(async () => {
    try {
      options = await api.getReferralCvOptions(companySlug, jobSlug);
      const first = options.tailored[0];
      if (first) {
        cvKind = 'built';
        cvId = first.id;
      }
    } catch {
      // Leave the choice to the two ways in; the backend re-validates on submit.
      options = { tailored: [], has_resume: false };
    }
  });

  // Uploading here is the profile's own upload: the file becomes the seeker's stored CV.
  async function upload(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    uploading = true;
    uploadError = null;
    try {
      await api.extractResumeProfile(file);
      const fresh = await api.getReferralCvOptions(companySlug, jobSlug);
      if (!fresh.has_resume) throw new Error('not stored');
      options = fresh;
      cvKind = 'original';
    } catch (err) {
      uploadError =
        err instanceof ApiError && err.status === 413 ? err.message : "Couldn't upload your CV. Please try again.";
    } finally {
      uploading = false;
    }
  }

  const tailoredTitle = $derived(options?.tailored.find((o) => o.id === cvId)?.title ?? '');

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 409) return 'You already have an active request for this company.';
      if (e.status === 422) return 'That CV can no longer be shared. Upload your CV or tailor one again.';
      if (e.status === 429) return "You've reached today's referral request limit.";
      if (e.status === 401) return 'Please sign in to request a referral.';
    }
    return 'Something went wrong. Please try again.';
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    error = null;
    submitting = true;
    const input: ReferralRequestInput = {
      company_slug: companySlug,
      job_slug: jobSlug,
      cv_kind: cvKind ?? 'original',
      cv_id: cvKind === 'built' && cvId !== null ? cvId : undefined,
      linkedin_url: linkedinUrl.trim(),
      contact_telegram: contactTelegram.trim() || undefined,
      contact_email: contactEmail.trim() || undefined,
      note: note.trim() || undefined,
    };
    try {
      await api.createReferralRequest(input);
      done = true;
      onSent?.();
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog bind:open title={`Ask for a referral · ${companyName}`} class="sm:max-w-md">

    {#if done}
      <div class="flex flex-col items-center gap-3 py-4 text-center">
        <span class="flex size-10 items-center justify-center rounded-full bg-secondary">
          <Check class="size-5" />
        </span>
        <p class="text-sm">Request sent — the referrer will reach out if interested.</p>
        <Button variant="outline" onclick={onClose} class="mt-1">Close</Button>
      </div>
    {:else}
      <form class="flex flex-col gap-4" onsubmit={submit}>
        <fieldset class="flex flex-col gap-2">
          <legend class="mb-1 text-sm font-medium">Which CV to share?</legend>
          {#if !options}
            <p class="text-sm text-muted-foreground">Loading…</p>
          {:else if cvKind === 'built' && cvId}
            <p class="flex items-center gap-2 text-sm">
              <Paperclip class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              <span>Attached: <span class="font-medium">{tailoredTitle}</span></span>
            </p>
            {#if options.tailored.length > 1}
              <!-- Only CVs tailored for this company's vacancies — never another company's. -->
              <select
                bind:value={cvId}
                aria-label="CV tailored for {companyName}"
                class="w-full min-w-0 rounded-md border border-border bg-background px-2 py-1 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {#each options.tailored as cv (cv.id)}
                  <option value={cv.id}>{cv.title}</option>
                {/each}
              </select>
            {/if}
            {#if options.has_resume}
              <button type="button" class="w-fit text-xs underline" onclick={() => (cvKind = 'original')}>
                Share my uploaded CV instead
              </button>
            {/if}
          {:else if cvKind === 'original'}
            <p class="flex items-center gap-2 text-sm">
              <Paperclip class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              <span>Attached: <span class="font-medium">your uploaded CV</span></span>
            </p>
            {#if options.tailored[0]}
              <button
                type="button"
                class="w-fit text-xs underline"
                onclick={() => {
                  cvKind = 'built';
                  cvId = options?.tailored[0]?.id ?? null;
                }}
              >
                Share the CV tailored for {companyName} instead
              </button>
            {/if}
          {:else}
            <div class="flex flex-col gap-2">
              {#if options.has_resume}
                <Button type="button" variant="outline" class="gap-1.5" onclick={() => (cvKind = 'original')}>
                  <Paperclip class="size-4" aria-hidden="true" /> Use my uploaded CV
                </Button>
              {:else}
                <Button
                  type="button"
                  variant="outline"
                  class="gap-1.5"
                  disabled={uploading}
                  onclick={() => fileInput?.click()}
                >
                  <FileUp class="size-4" aria-hidden="true" />
                  {uploading ? 'Uploading…' : 'Upload my CV'}
                </Button>
                <input
                  bind:this={fileInput}
                  type="file"
                  accept="application/pdf"
                  class="hidden"
                  onchange={upload}
                  aria-label="Upload my CV (PDF, up to {RESUME_MAX_MB} MB)"
                />
              {/if}
              {#if onTailor}
                <Button type="button" variant="primary" class="gap-1.5" onclick={onTailor}>
                  <Sparkles class="size-4" aria-hidden="true" /> Tailor my CV for this vacancy
                </Button>
              {/if}
            </div>
            {#if uploadError}
              <p class="text-sm text-destructive">{uploadError}</p>
            {/if}
          {/if}
        </fieldset>

        <FormField
          label="Your LinkedIn profile"
          error={linkedinUrl.trim() !== '' && !linkedinValid
            ? 'Enter a full linkedin.com/in/… profile URL.'
            : undefined}
          hint="The referrer vets you before reaching out."
        >
          {#snippet children({ id, describedBy, invalid })}
            <input
              {id}
              type="url"
              bind:value={linkedinUrl}
              placeholder="https://linkedin.com/in/your-handle"
              aria-invalid={invalid}
              aria-describedby={describedBy}
              class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring aria-[invalid=true]:border-destructive"
            />
          {/snippet}
        </FormField>

        <div class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">
            How should the referrer reach you?
            <span class="text-xs font-normal text-muted-foreground">(at least one)</span>
          </span>
          <div class="grid grid-cols-2 gap-2">
            <input
              type="text"
              bind:value={contactTelegram}
              placeholder="Telegram @handle"
              class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <input
              type="email"
              bind:value={contactEmail}
              placeholder="you@example.com"
              class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>

        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">
            Note <span class="text-xs font-normal text-muted-foreground">(optional)</span>
          </span>
          <textarea
            bind:value={note}
            rows="3"
            placeholder="Short intro — why you're a fit…"
            class="resize-y rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          ></textarea>
        </label>

        {#if error}
          <p class="text-sm text-destructive">{error}</p>
        {/if}

        <div class="flex justify-end gap-2">
          <Button type="button" variant="outline" onclick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={submitting || !canSubmit}>
            {submitting ? 'Sending…' : 'Send request'}
          </Button>
        </div>
      </form>
    {/if}
</Dialog>
