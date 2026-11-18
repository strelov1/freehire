<script lang="ts">
  import { Check, Copy, Reply, X } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { Button, lockScroll, unlockScroll } from '#lib/ui/index.js';
  import { api } from '#lib/api.js';
  import { ApiError, isPlanRefusal } from '#lib/api-core.js';
  import { gmailHref, mailtoHref } from '#lib/followup.js';
  import { errorMessage } from '#lib/utils.js';
  import { announce } from '#lib/announce.svelte.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import type { MailReplyDraft } from '#lib/types.js';
  import { messages } from './ReplyDraft.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  // A button and its dialog in one, so the inbox only places it. Each open is a new,
  // charged draft; the text is never stored, so closing the dialog discards it.
  let { emailId }: { emailId: number } = $props();

  let open = $state(false);
  let draft = $state.raw<MailReplyDraft | null>(null);
  let body = $state(''); // the candidate's edit of draft.body; the compose links carry it
  let error = $state<string | null>(null);
  let refused = $state(false); // a 402: the server's sentence, never a retry
  let offerPlans = $state(false);
  let copied = $state(false);
  // Bumped by every load and by closing, so a slower, earlier request cannot overwrite a newer draft.
  let generation = 0;

  const s = $derived(t(messages, locale()));
  const compose = $derived(draft ? { ...draft, body } : null);

  async function load() {
    const mine = ++generation;
    draft = null;
    error = null;
    refused = false;
    offerPlans = false;
    copied = false;
    try {
      const next = await api.draftEmailReply(emailId);
      if (mine !== generation) return;
      draft = next;
      body = next.body;
    } catch (e) {
      if (mine !== generation) return;
      refused = isPlanRefusal(e);
      // A paid account that spent today's allowance is told so; only a refusal carrying an
      // upgrade link is offered the plans.
      offerPlans = refused && e instanceof ApiError && typeof e.body?.upgrade_url === 'string';
      error = errorMessage(e, s.failed);
      announce(error, 'assertive');
    }
  }

  function close() {
    open = false;
    generation++;
  }

  function start() {
    open = true;
    void load();
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(body);
    } catch (e) {
      error = errorMessage(e, s.copyFailed);
      return;
    }
    copied = true;
    announce(s.copied);
  }

  $effect(() => {
    if (!open) return;
    lockScroll();
    return unlockScroll;
  });
</script>

<Button variant="outline" size="sm" onclick={start} class="gap-1.5">
  <Reply class="size-4" />
  {s.draftReply}
</Button>

<svelte:window onkeydown={(e) => open && e.key === 'Escape' && close()} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <button
      type="button"
      aria-label={s.closeAria}
      class="absolute inset-0 animate-modal-fade bg-black/50"
      onclick={close}
    ></button>
    <div
      role="dialog"
      aria-modal="true"
      aria-label={s.heading}
      class="relative z-10 flex animate-modal-pop max-h-full w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-xl"
      {@attach focusTrap()}
    >
      <div class="flex items-center gap-3 border-b border-border p-5">
        <h2 class="flex-1 text-lg font-semibold leading-tight">{s.heading}</h2>
        <button
          type="button"
          onclick={close}
          aria-label={s.close}
          class="-mr-1 rounded-full p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-5" />
        </button>
      </div>

      <div class="flex-1 overflow-y-auto overscroll-contain p-5">
        {#if refused}
          <p class="text-sm">{error}</p>
          {#if offerPlans}
            <a
              href={resolve('/pricing')}
              class="mt-2 inline-block text-sm font-medium text-brand-strong hover:underline"
            >
              {s.viewPlans}
            </a>
          {/if}
        {:else if error && !draft}
          <p class="text-sm text-destructive">{error}</p>
        {:else if !draft}
          <p class="text-sm text-muted-foreground">{s.building}</p>
        {:else}
          <dl class="flex flex-col gap-2 text-sm">
            <div class="flex gap-2">
              <dt class="w-16 shrink-0 text-muted-foreground">{s.to}</dt>
              <dd class="min-w-0 flex-1 truncate">
                {draft.recipient_name ? `${draft.recipient_name} <${draft.recipient}>` : draft.recipient}
              </dd>
            </div>
            <div class="flex gap-2">
              <dt class="w-16 shrink-0 text-muted-foreground">{s.subject}</dt>
              <dd class="min-w-0 flex-1 font-medium">{draft.subject}</dd>
            </div>
          </dl>
          <textarea
            bind:value={body}
            aria-label={s.bodyLabel}
            rows="10"
            class="mt-4 w-full resize-y rounded-lg border border-border bg-muted/40 p-3 text-sm leading-relaxed"
          ></textarea>
          <p class="mt-3 text-xs text-muted-foreground">{s.note}</p>
          {#if error}<p class="mt-2 text-xs text-destructive">{error}</p>{/if}
        {/if}
      </div>

      <div class="flex flex-wrap items-center gap-2 border-t border-border p-4">
        {#if compose}
          <!-- eslint-disable svelte/no-navigation-without-resolve -- mail handoffs, not internal routes -->
          <span class="flex items-center gap-1 text-xs text-muted-foreground">
            {s.openIn}
            <a
              href={gmailHref(compose)}
              target="_blank"
              rel="noopener noreferrer"
              class="font-medium text-brand-strong hover:underline">Gmail</a
            >
            <span aria-hidden="true">·</span>
            <a href={mailtoHref(compose)} class="font-medium text-brand-strong hover:underline">{s.mailApp}</a>
          </span>
          <!-- eslint-enable svelte/no-navigation-without-resolve -->
        {/if}
        <span class="ml-auto flex gap-2">
          {#if draft || (error && !refused)}
            <Button variant="outline" onclick={load}>{s.redraft}</Button>
          {/if}
          <Button variant="primary" onclick={copy} disabled={!draft} class="gap-1.5">
            {#if copied}
              <Check class="size-4" /> {s.copied}
            {:else}
              <Copy class="size-4" /> {s.copy}
            {/if}
          </Button>
        </span>
      </div>
    </div>
  </div>
{/if}
