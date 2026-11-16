<script lang="ts">
  import type { InboxMessage } from '#lib/api.js';
  import { avatarColor, avatarInitials } from '#lib/avatar.js';
  import { statusLabel } from '#lib/emailStatus.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { timeAgo } from '#lib/utils.js';
  import StatusChip from '#lib/components/StatusChip.svelte';
  import { messages } from './InboxView.messages';

  // One message in the inbox list: sender, time, subject, snippet and its status.
  let { m, selected, onOpen }: { m: InboxMessage; selected: boolean; onOpen: () => void } = $props();

  const s = $derived(t(messages, locale()));
</script>

<button
  type="button"
  onclick={onOpen}
  aria-current={selected}
  class="flex w-full items-start gap-3 rounded-xl border p-3 text-left transition-colors {selected
    ? 'border-brand-ring bg-brand-muted/60'
    : 'border-transparent hover:border-border hover:bg-accent'}"
>
  <div
    class="mt-0.5 flex h-9 w-9 shrink-0 select-none items-center justify-center rounded-full text-xs font-semibold text-white"
    style="background-color: {avatarColor(m.from_addr || m.from_name)}"
  >
    {avatarInitials(m.from_name, m.from_addr)}
  </div>
  <div class="min-w-0 flex-1">
    <div class="flex items-baseline gap-2">
      {#if !m.read}
        <span class="sr-only">{s.unreadDotAria}</span>
        <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-brand" aria-hidden="true"></span>
      {/if}
      <span
        class="min-w-0 flex-1 truncate text-sm {m.read
          ? 'font-medium text-foreground/90'
          : 'font-semibold text-foreground'}"
      >
        {m.direction === 'outgoing' ? `${s.sent} · ${m.to_addrs || m.from_addr}` : m.from_name || m.from_addr}
      </span>
      <span class="shrink-0 text-[11px] text-muted-foreground">{timeAgo(m.received_at, locale())}</span>
    </div>
    <div class="mt-0.5 truncate text-sm {m.read ? 'text-muted-foreground' : 'text-foreground'}">
      {m.subject || s.noSubject}
    </div>
    {#if m.snippet}
      <div class="mt-0.5 truncate text-xs text-muted-foreground">{m.snippet}</div>
    {/if}
    {#if statusLabel(m.status_signal, locale()) || m.linked_slug}
      <div class="mt-1 flex items-center gap-1">
        <StatusChip signal={m.status_signal} class="text-[10px] leading-4" />
        {#if m.linked_slug}
          <span class="truncate text-[10px] text-muted-foreground">· {m.linked_company}</span>
        {:else if m.suggested_slug}
          <span class="text-[10px] text-brand-strong">· {s.suggestedTag}</span>
        {/if}
      </div>
    {/if}
  </div>
</button>
