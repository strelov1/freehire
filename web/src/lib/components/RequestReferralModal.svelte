<script lang="ts">
  import { onMount } from 'svelte';
  import { Check, X } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import type { CvTailoredItem } from '$lib/cv';
  import type { ReferralRequestInput } from '$lib/types';
  import { Button } from '$lib/ui';
  import { isLinkedInUrl } from '$lib/utils';

  // The parent owns open/close; this component owns the request form. jobId is the
  // optional source-vacancy context recorded with the request.
  let {
    companySlug,
    companyName,
    jobId,
    onClose,
    onSent,
  }: {
    companySlug: string;
    companyName: string;
    jobId?: number;
    onClose: () => void;
    onSent?: () => void;
  } = $props();

  let cvKind = $state<'original' | 'built'>('original');
  let cvId = $state<number | null>(null);
  let tailored = $state<CvTailoredItem[]>([]);
  let hasResume = $state(true);
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
      (cvKind === 'original' ? hasResume : cvId !== null),
  );

  onMount(async () => {
    // Load the CV options: the stored-résumé presence and the tailored CVs. A load
    // failure just leaves the defaults — the backend re-validates on submit.
    const [resume, cvs] = await Promise.allSettled([api.getResume(), api.listCvs()]);
    if (resume.status === 'fulfilled') hasResume = resume.value.present;
    if (cvs.status === 'fulfilled') tailored = cvs.value;
    // If there is no stored résumé but tailored CVs exist, start on the usable option.
    if (!hasResume && tailored.length > 0) {
      cvKind = 'built';
      cvId = tailored[0]?.id ?? null;
    }
  });

  function pickBuilt() {
    cvKind = 'built';
    if (cvId === null) cvId = tailored[0]?.id ?? null;
  }

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 409) return 'You already have an active request for this company.';
      if (e.status === 422) return 'Upload a CV first, or pick a tailored one.';
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
      job_id: jobId,
      cv_kind: cvKind,
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

<svelte:window onkeydown={(e) => e.key === 'Escape' && onClose()} />

<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
  <button type="button" aria-label="Close dialog" class="absolute inset-0 bg-black/50" onclick={onClose}
  ></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-label="Ask for a referral"
    class="relative w-full max-w-md rounded-lg border border-border bg-background p-6 shadow-lg"
  >
    <div class="mb-4 flex items-center justify-between gap-4">
      <h2 class="text-base font-semibold tracking-tight">Ask for a referral · {companyName}</h2>
      <button
        type="button"
        aria-label="Close"
        onclick={onClose}
        class="text-muted-foreground hover:text-foreground"
      >
        <X class="size-5" />
      </button>
    </div>

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

          <label
            class={[
              'flex items-center gap-3 rounded-md border px-3 py-2.5 text-sm transition-colors',
              cvKind === 'original' ? 'border-brand bg-brand-muted' : 'border-border',
              !hasResume && 'opacity-50',
            ]}
          >
            <input
              type="radio"
              name="cvkind"
              checked={cvKind === 'original'}
              disabled={!hasResume}
              onchange={() => (cvKind = 'original')}
            />
            <span class="font-medium">My uploaded CV</span>
            {#if !hasResume}
              <span class="ml-auto text-xs text-muted-foreground">none uploaded</span>
            {/if}
          </label>

          <label
            class={[
              'flex items-center gap-3 rounded-md border px-3 py-2.5 text-sm transition-colors',
              cvKind === 'built' ? 'border-brand bg-brand-muted' : 'border-border',
              tailored.length === 0 && 'opacity-50',
            ]}
          >
            <input
              type="radio"
              name="cvkind"
              checked={cvKind === 'built'}
              disabled={tailored.length === 0}
              onchange={pickBuilt}
            />
            <span class="font-medium">Tailored CV</span>
            {#if tailored.length > 0}
              <select
                bind:value={cvId}
                onclick={(e) => e.stopPropagation()}
                class="ml-auto max-w-[55%] rounded-md border border-border bg-background px-2 py-1 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {#each tailored as cv (cv.id)}
                  <option value={cv.id}>{cv.job_company ? `${cv.title} — ${cv.job_company}` : cv.title}</option>
                {/each}
              </select>
            {:else}
              <span class="ml-auto text-xs text-muted-foreground">none yet</span>
            {/if}
          </label>
        </fieldset>

        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">Your LinkedIn profile</span>
          <input
            type="url"
            bind:value={linkedinUrl}
            placeholder="https://linkedin.com/in/your-handle"
            aria-invalid={linkedinUrl.trim() !== '' && !linkedinValid}
            class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring aria-[invalid=true]:border-destructive"
          />
          {#if linkedinUrl.trim() !== '' && !linkedinValid}
            <span class="text-xs text-destructive">Enter a full linkedin.com/in/… profile URL.</span>
          {:else}
            <span class="text-xs text-muted-foreground">The referrer vets you before reaching out.</span>
          {/if}
        </label>

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
  </div>
</div>
