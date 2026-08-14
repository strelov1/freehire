<script lang="ts">
  import { onMount } from 'svelte';
  import { Star } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { communityFormError } from '$lib/community';
  import { companyFeedbackTypes, maxFeedbackBodyLength } from '$lib/companyFeedback';
  import type { CompanyFeedbackSummary } from '$lib/types';
  import { Button, ConfirmDialog, Dialog } from '$lib/ui';

  // Company feedback: a 1-5 star rating + a category + free text, one per
  // (caller, company), editable by resubmitting. The parent owns open/close;
  // this component owns the form within, following ReportDialog's shape.
  // `onSaved` fires after a successful submit/delete (not on plain cancel)
  // with the company's freshly recomputed counters, so the parent can update
  // its own rating summary directly — no follow-up fetch, so no window where
  // that fetch can itself fail and leave a stale badge on screen.
  let {
    slug,
    onClose,
    onSaved,
  }: { slug: string; onClose: () => void; onSaved?: (summary: CompanyFeedbackSummary) => void } = $props();

  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  // Prefilled from the caller's existing feedback, if any — 'edit' shows the
  // delete action, 'create' does not.
  let mode = $state<'loading' | 'create' | 'edit'>('loading');
  let rating = $state(0);
  let hoverRating = $state(0);
  let feedbackType = $state('');
  let body = $state('');
  let submitting = $state(false);
  let confirmDeleteOpen = $state(false);
  let error = $state<string | null>(null);

  onMount(async () => {
    try {
      const mine = await api.getMyCompanyFeedback(slug);
      if (mine) {
        rating = mine.rating;
        feedbackType = mine.feedback_type;
        body = mine.body;
        mode = 'edit';
      } else {
        mode = 'create';
      }
    } catch {
      // A prefill failure just starts a blank form — the server re-validates
      // (and re-upserts in place) on submit regardless.
      mode = 'create';
    }
  });

  const canSubmit = $derived(rating > 0 && feedbackType !== '' && body.trim() !== '' && !submitting);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    submitting = true;
    error = null;
    try {
      const saved = await api.upsertCompanyFeedback(slug, { rating, feedback_type: feedbackType, body: body.trim() });
      onSaved?.(saved.company);
      open = false;
    } catch (err) {
      error = communityFormError(err);
      submitting = false;
    }
  }

  async function remove() {
    try {
      const summary = await api.deleteCompanyFeedback(slug);
      onSaved?.(summary);
      open = false;
    } catch (err) {
      // null means a 401 already opened the sign-in dialog — nothing left to say here,
      // so the confirm dialog just closes and lets that dialog take over.
      const message = communityFormError(err);
      if (message) throw new Error(message, { cause: err });
    }
  }
</script>

<Dialog bind:open title={mode === 'edit' ? 'Edit your feedback' : 'Leave feedback'} class="max-w-md">
  {#if mode === 'loading'}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else}
    <form class="flex flex-col gap-4" onsubmit={submit}>
      <fieldset class="flex flex-col gap-1.5">
        <legend class="text-sm font-medium">Rating</legend>
        <div class="flex items-center gap-1" onmouseleave={() => (hoverRating = 0)} role="group">
          {#each [1, 2, 3, 4, 5] as value (value)}
            <button
              type="button"
              class="p-0.5 text-muted-foreground transition-colors hover:text-warning-strong"
              class:text-warning-strong={value <= (hoverRating || rating)}
              aria-pressed={value === rating}
              onmouseenter={() => (hoverRating = value)}
              onclick={() => (rating = value)}
              aria-label={`${value} star${value === 1 ? '' : 's'}`}
            >
              <Star class="size-6" fill={value <= (hoverRating || rating) ? 'currentColor' : 'none'} />
            </button>
          {/each}
        </div>
      </fieldset>

      <label class="flex flex-col gap-1.5 text-sm">
        <span class="font-medium">Category</span>
        <select
          bind:value={feedbackType}
          class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <option value="" disabled>Choose a category…</option>
          {#each companyFeedbackTypes as t (t.value)}
            <option value={t.value}>{t.label}</option>
          {/each}
        </select>
      </label>

      <label class="flex flex-col gap-1.5 text-sm">
        <span class="font-medium">Your feedback</span>
        <textarea
          bind:value={body}
          rows="5"
          maxlength={maxFeedbackBodyLength}
          placeholder="Share your experience…"
          class="resize-y rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        ></textarea>
      </label>

      {#if error}
        <p role="alert" class="text-sm text-destructive">{error}</p>
      {/if}

      <div class="flex items-center justify-between gap-2">
        {#if mode === 'edit'}
          <Button
            type="button"
            variant="ghost"
            disabled={submitting}
            onclick={() => (confirmDeleteOpen = true)}
          >
            Delete my feedback
          </Button>
        {:else}
          <span></span>
        {/if}
        <Button type="submit" variant="primary" disabled={!canSubmit}>
          {submitting ? 'Saving…' : mode === 'edit' ? 'Save changes' : 'Post feedback'}
        </Button>
      </div>
    </form>
  {/if}
</Dialog>

<ConfirmDialog
  bind:open={confirmDeleteOpen}
  title="Delete your feedback on this company?"
  description="This cannot be undone."
  confirmLabel="Delete"
  variant="destructive"
  onConfirm={remove}
/>
