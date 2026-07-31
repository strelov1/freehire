<script lang="ts">
  import { Check, Copy, Clock, X } from '@lucide/svelte';
  import { Button } from '$lib/ui';
  import { api } from '$lib/api';
  import { clipboardText } from '$lib/followup';
  import { errorMessage } from '$lib/utils';
  import { focusTrap } from '$lib/actions/focusTrap';
  import type { FollowUpDraft } from '$lib/types';

  // The parent owns open/close. onrecorded reports the chase back so the board can
  // stamp the card without refetching the whole listing.
  let {
    slug,
    company,
    onclose,
    onrecorded,
  }: {
    slug: string;
    company: string;
    onclose: () => void;
    onrecorded: (at: string) => void;
  } = $props();

  let draft = $state.raw<FollowUpDraft | null>(null); // an API payload, only ever reassigned
  let error = $state<string | null>(null);
  let copied = $state(false);

  // Fetch once per mount; the parent keys this dialog by slug, so the prop never
  // changes underneath it.
  $effect(() => {
    void load();
  });

  async function load() {
    try {
      draft = await api.getFollowUpDraft(slug);
    } catch (e) {
      error = errorMessage(e, "Couldn't build the follow-up draft.");
    }
  }

  // Copying IS the record: the candidate takes the text away to send it, and asking
  // for a second confirming click is the friction this feature exists to remove.
  // A failed POST does not undo the copy — the text is already on the clipboard, so
  // the button stays "Copied" and only the chase mark is lost.
  async function copy() {
    if (!draft) return;
    try {
      await navigator.clipboard.writeText(clipboardText(draft));
    } catch (e) {
      error = errorMessage(e, "Couldn't copy to the clipboard.");
      return;
    }
    copied = true;
    try {
      await api.recordFollowUp(slug);
      onrecorded(new Date().toISOString());
    } catch (e) {
      error = errorMessage(e, "Copied, but couldn't record the follow-up.");
    }
  }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
  <button type="button" aria-label="Close dialog" class="absolute inset-0 bg-black/50" onclick={onclose}></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-label="Follow up"
    class="relative z-10 flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-xl"
    {@attach focusTrap()}
  >
    <div class="flex items-start gap-3 border-b border-border p-5">
      <div class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-amber-100 dark:bg-amber-950/50">
        <Clock class="size-5 text-amber-600 dark:text-amber-400" />
      </div>
      <div class="min-w-0 flex-1">
        <h2 class="text-lg font-semibold leading-tight">Follow up</h2>
        <p class="mt-0.5 truncate text-sm text-muted-foreground">
          {#if draft}
            {company || 'This application'} — no reply for {draft.days_silent} days
          {:else}
            {company || 'This application'}
          {/if}
        </p>
      </div>
      <button
        type="button"
        onclick={onclose}
        aria-label="Close"
        class="-mr-1 rounded-full p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X class="size-5" />
      </button>
    </div>

    <div class="flex-1 overflow-y-auto p-5">
      {#if error && !draft}
        <p class="text-sm text-destructive">{error}</p>
      {:else if !draft}
        <p class="text-sm text-muted-foreground">Building your draft…</p>
      {:else}
        <dl class="flex flex-col gap-2 text-sm">
          {#if draft.recipient}
            <div class="flex gap-2">
              <dt class="w-16 shrink-0 text-muted-foreground">To</dt>
              <dd class="min-w-0 flex-1 truncate">
                {draft.recipient_name ? `${draft.recipient_name} <${draft.recipient}>` : draft.recipient}
              </dd>
            </div>
          {/if}
          <div class="flex gap-2">
            <dt class="w-16 shrink-0 text-muted-foreground">Subject</dt>
            <dd class="min-w-0 flex-1 font-medium">{draft.subject}</dd>
          </div>
        </dl>
        <pre
          class="mt-4 whitespace-pre-wrap rounded-lg border border-border bg-muted/40 p-3 font-sans text-sm leading-relaxed">{draft.body}</pre>
        <p class="mt-3 text-xs text-muted-foreground">
          Send it from your own mail — freehire never writes to anyone on your behalf.
        </p>
        {#if error}
          <p class="mt-2 text-xs text-destructive">{error}</p>
        {/if}
      {/if}
    </div>

    <div class="flex justify-end gap-2 border-t border-border p-4">
      <Button variant="outline" onclick={onclose}>Close</Button>
      <Button variant="primary" onclick={copy} disabled={!draft} class="gap-1.5">
        {#if copied}
          <Check class="size-4" /> Copied
        {:else}
          <Copy class="size-4" /> Copy draft
        {/if}
      </Button>
    </div>
  </div>
</div>
