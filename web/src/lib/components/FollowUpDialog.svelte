<script lang="ts">
  import { Check, Copy } from '@lucide/svelte';
  import { Button, Dialog } from '#lib/ui/index.js';
  import { api } from '#lib/api.js';
  import { clipboardText, gmailHref, mailtoHref } from '#lib/followup.js';
  import { errorMessage } from '#lib/utils.js';
  import { announce } from '#lib/announce.svelte.js';
  import type { FollowUpDraft } from '#lib/types.js';
  import { messages } from './FollowUpDialog.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';

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

  const s = $derived(t(messages, locale()));
  const who = $derived(company || s.thisApplication);

  // Fetch once per mount; the parent keys this dialog by slug, so the prop never
  // changes underneath it.
  $effect(() => {
    void load();
  });

  function fail(e: unknown, fallback: string) {
    error = errorMessage(e, fallback);
    announce(error, 'assertive');
  }

  async function load() {
    try {
      draft = await api.getFollowUpDraft(slug);
    } catch (e) {
      fail(e, s.loadFailed);
    }
  }

  // Taking the draft away IS the record — by clipboard or by compose link, both hand
  // it to the candidate, and asking for a second confirming click reintroduces the
  // friction this feature removes. A failed POST does not undo the handoff: the draft
  // is already gone, so only the chase mark is lost and the message says so.
  async function record() {
    try {
      await api.recordFollowUp(slug);
      onrecorded(new Date().toISOString());
    } catch (e) {
      fail(e, s.recordFailed);
    }
  }

  async function copy() {
    if (!draft) return;
    try {
      await navigator.clipboard.writeText(clipboardText(draft));
    } catch (e) {
      fail(e, s.copyFailed);
      return;
    }
    copied = true;
    announce(s.copied);
    await record();
  }

  // The parent mounts this component to open it and unmounts on onclose, so the
  // dialog is open for its whole life. Dialog owns the closing — Escape, the
  // backdrop and its own button all land on `open`, and this reports it upward
  // rather than each affordance calling onclose itself.
  let open = $state(true);
  $effect(() => {
    if (!open) onclose();
  });
</script>

<Dialog
  bind:open
  title={s.heading}
  description={`${who}${draft ? ` — ${format(plural(locale(), draft.days_silent, s.noReply), { days: String(draft.days_silent) })}` : ''}`}
  class="sm:max-w-lg"
>
  <div class="flex flex-col gap-4">
    {#if error && !draft}
      <p class="text-sm text-destructive">{error}</p>
    {:else if !draft}
      <p class="text-sm text-muted-foreground">{s.building}</p>
    {:else}
      <dl class="flex flex-col gap-2 text-sm">
        {#if draft.recipient}
          <div class="flex gap-2">
            <dt class="w-16 shrink-0 text-muted-foreground">{s.to}</dt>
            <dd class="min-w-0 flex-1 truncate">
              {draft.recipient_name ? `${draft.recipient_name} <${draft.recipient}>` : draft.recipient}
            </dd>
          </div>
        {/if}
        <div class="flex gap-2">
          <dt class="w-16 shrink-0 text-muted-foreground">{s.subject}</dt>
          <dd class="min-w-0 flex-1 font-medium">{draft.subject}</dd>
        </div>
      </dl>
      <pre
        class="whitespace-pre-wrap rounded-lg border border-border bg-muted/40 p-3 font-sans text-sm leading-relaxed">{draft.body}</pre>
      <p class="mt-3 text-xs text-muted-foreground">
        {s.privacyNote}
      </p>
      {#if error}
        <p class="mt-2 text-xs text-destructive">{error}</p>
      {/if}
    {/if}
    <div class="flex flex-wrap items-center gap-2">
      {#if draft}
        <!-- Both links open a compose window in the candidate's own client, prefilled.
             Neither sends; both record the chase on the way out. Gmail is offered
             beside mailto because a browser with no registered mail handler does
             nothing at all on a mailto click. -->
        <!-- eslint-disable svelte/no-navigation-without-resolve -- mail handoffs, not internal routes -->
        <span class="flex items-center gap-1 text-xs text-muted-foreground">
          {s.openIn}
          <a
            href={gmailHref(draft)}
            target="_blank"
            rel="noopener noreferrer"
            onclick={record}
            class="font-medium text-brand-strong hover:underline">Gmail</a
          >
          <span aria-hidden="true">·</span>
          <a href={mailtoHref(draft)} onclick={record} class="font-medium text-brand-strong hover:underline">
            {s.mailApp}
          </a>
        </span>
        <!-- eslint-enable svelte/no-navigation-without-resolve -->
      {/if}
      <span class="ml-auto flex gap-2">
        <Button type="button" variant="outline" onclick={() => (open = false)}>{s.close}</Button>
        <Button variant="primary" onclick={copy} disabled={!draft} class="gap-1.5">
          {#if copied}
            <Check class="size-4" /> {s.copied}
          {:else}
            <Copy class="size-4" /> {s.copyDraft}
          {/if}
        </Button>
      </span>
    </div>
  </div>
</Dialog>
