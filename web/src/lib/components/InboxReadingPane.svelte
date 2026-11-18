<script lang="ts">
  import { resolve } from '$app/paths';
  import { Trash2 } from '@lucide/svelte';
  import type { EmailBody } from '#lib/api.js';
  import { avatarColor, avatarInitials } from '#lib/avatar.js';
  import StatusChip from '#lib/components/StatusChip.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { inboxLinkState, type LastUnlinked } from '#lib/inboxLink.js';
  import { timeAgo } from '#lib/utils.js';
  import ApplicationLinkPicker from './ApplicationLinkPicker.svelte';
  import ReplyDraft from './ReplyDraft.svelte';
  import { messages } from './InboxView.messages';

  // The inbox's reading pane for one message: who sent it, the link to an application
  // (linked, suggested, or picked by hand, with a one-click Undo), and the body. Untrusted
  // sender HTML stays in a sandboxed iframe. InboxView owns the state every action changes.
  let {
    msg,
    lastUnlinked,
    trackedApps,
    trackedLoading,
    gmailUrl,
    ondelete,
    onunlink,
    onconfirm,
    onreject,
    onundo,
    onpick,
  }: {
    msg: EmailBody;
    lastUnlinked: LastUnlinked | null;
    trackedApps: { slug: string; company: string; title?: string }[];
    trackedLoading: boolean;
    /** The message in Gmail's web UI, for a Gmail-sourced one. */
    gmailUrl: string;
    ondelete: () => void;
    onunlink: () => void;
    onconfirm: () => void;
    onreject: () => void;
    onundo: () => void;
    onpick: (slug: string) => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  const linkState = $derived(inboxLinkState(msg, lastUnlinked));
</script>

<div class="flex shrink-0 items-start gap-3">
  <div
    class="flex h-10 w-10 shrink-0 select-none items-center justify-center rounded-full text-sm font-semibold text-white"
    style="background-color: {avatarColor(msg.from_addr || msg.from_name)}"
  >
    {avatarInitials(msg.from_name, msg.from_addr)}
  </div>
  <div class="min-w-0 flex-1">
    <h2 class="text-base font-semibold leading-snug tracking-tight">{msg.subject || s.noSubject}</h2>
    <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
      <span class="font-medium text-foreground/80">{msg.from_name || msg.from_addr}</span>
      {#if msg.from_name}
        <span aria-hidden="true">·</span>
        <span class="truncate">{msg.from_addr}</span>
      {/if}
      {#if msg.direction === 'outgoing'}<span>{s.sent}</span>{/if}
      <span class="ml-auto shrink-0">{timeAgo(msg.received_at, locale())}</span>
    </div>
  </div>
  <button
    type="button"
    onclick={ondelete}
    title={s.deleteTitle}
    aria-label={s.deleteMessageAria}
    class="shrink-0 rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
  >
    <Trash2 class="h-4 w-4" />
  </button>
</div>

{#if msg.to_addrs}<div class="mt-2 break-words text-xs text-muted-foreground">
    {s.to}: {msg.to_addrs}
  </div>{/if}
{#if msg.cc_addrs}<div class="mt-1 break-words text-xs text-muted-foreground">
    {s.cc}: {msg.cc_addrs}
  </div>{/if}
<div class="mt-3 flex shrink-0 flex-wrap items-center gap-2">
  {#if msg.direction !== 'outgoing'}<StatusChip signal={msg.status_signal} /><ReplyDraft emailId={msg.id} />{/if}
  {#if linkState === 'linked' && msg.linked_slug}
    <a
      href={resolve('/my/tracking/[id]', { id: msg.linked_slug })}
      class="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-0.5 text-xs text-muted-foreground transition-colors hover:border-brand-ring hover:text-foreground"
    >
      {format(s.linkedTo, { company: msg.linked_company || s.linkedFallback })} ↗
    </a>
    <button
      type="button"
      onclick={onunlink}
      class="relative text-xs text-muted-foreground after:absolute after:-inset-1 hover:text-destructive"
      >{s.unlink}</button
    >
  {:else if linkState === 'suggested'}
    <span
      class="inline-flex items-center gap-2 rounded-full border border-brand-ring/50 bg-brand-muted/40 px-2.5 py-0.5 text-xs"
    >
      {s.looksLike} <span class="font-medium">{msg.suggested_company || s.suggestedFallback}</span>
      <button
        type="button"
        onclick={onconfirm}
        class="relative font-medium text-brand-strong after:absolute after:-inset-1 hover:underline">{s.link}</button
      >
      <span aria-hidden="true">·</span>
      <button
        type="button"
        onclick={onreject}
        class="relative text-muted-foreground after:absolute after:-inset-1 hover:text-foreground">{s.notThis}</button
      >
    </span>
  {:else}
    {#if linkState === 'undo'}
      <span class="inline-flex items-center gap-2 text-xs text-muted-foreground">
        {s.unlinked}
        <span aria-hidden="true">·</span>
        <button
          type="button"
          onclick={onundo}
          class="relative font-medium text-brand-strong after:absolute after:-inset-1 hover:underline">{s.undo}</button
        >
      </span>
    {/if}
    <ApplicationLinkPicker applications={trackedApps} loading={trackedLoading} {onpick} />
  {/if}
</div>

{#if msg.source === 'gmail'}
  <div class="mt-2 flex shrink-0 justify-end">
    <!-- eslint-disable svelte/no-navigation-without-resolve -- external Gmail deep-link, not an internal route -->
    <a
      href={gmailUrl}
      target="_blank"
      rel="noopener noreferrer"
      class="text-xs font-medium text-brand-strong hover:underline"
      ><!-- eslint-enable svelte/no-navigation-without-resolve -->
      {s.openInGmail} ↗
    </a>
  </div>
{/if}

<hr class="my-4 shrink-0 border-border" />

{#if msg.body_html}
  <!-- Untrusted sender HTML isolated in a sandboxed iframe (no scripts/forms/navigation). -->
  <iframe
    title={s.messageBodyTitle}
    sandbox=""
    srcdoc={msg.body_html}
    class="min-h-0 w-full flex-1 rounded-md border border-border bg-white"
  ></iframe>
{:else}
  <pre
    class="min-h-0 flex-1 overflow-y-auto whitespace-pre-wrap font-sans text-sm leading-relaxed">{msg.body_text}</pre>
{/if}
