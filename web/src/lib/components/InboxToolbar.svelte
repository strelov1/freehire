<script lang="ts">
  import { CheckCheck, Mail, RefreshCw, Search } from '@lucide/svelte';
  import type { InboxSource } from '#lib/api.js';
  import { statusLabels } from '#lib/emailStatus.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './InboxView.messages';

  // The inbox toolbar: the account switcher, the label filter and the search, then the
  // unread toggle, mark-all-read and refresh.
  let {
    sourceOptions,
    showSources,
    source,
    onSetSource,
    label = $bindable(),
    onLabelChange,
    search = $bindable(),
    onSearchInput,
    unread,
    onToggleUnread,
    onMarkAllRead,
    markingAll,
    onRefresh,
    refreshing,
  }: {
    sourceOptions: { value: InboxSource; label: string }[];
    showSources: boolean;
    source: InboxSource;
    onSetSource: (source: InboxSource) => void;
    label: string;
    onLabelChange: () => void;
    search: string;
    onSearchInput: () => void;
    unread: boolean;
    onToggleUnread: () => void;
    onMarkAllRead: () => void;
    markingAll: boolean;
    onRefresh: () => void;
    refreshing: boolean;
  } = $props();

  const s = $derived(t(messages, locale()));
  // The dropdown offers only signals with a human label (drops 'other' → blank).
  const LABEL_OPTIONS = $derived(Object.entries(statusLabels(locale())).filter(([, l]) => l !== ''));
</script>

<!-- Toolbar: account switcher + label filter + search on the left; a compact
     icon cluster (unread filter, mark-all-read, refresh) on the right. -->
<div class="flex flex-wrap items-center gap-2">
  {#if showSources}
    <div class="flex gap-1 rounded-lg border border-border p-1 text-sm">
      {#each sourceOptions as opt (opt.value)}
        <button
          type="button"
          onclick={() => onSetSource(opt.value as InboxSource)}
          class="rounded px-3 py-1 transition-colors {source === opt.value
            ? 'bg-secondary font-medium text-foreground'
            : 'text-muted-foreground hover:text-foreground'}"
        >
          {opt.label}
        </button>
      {/each}
    </div>
  {/if}

  <select
    bind:value={label}
    onchange={onLabelChange}
    aria-label={s.filterByLabelAria}
    class="rounded-lg border border-border bg-background py-2 pl-3 pr-8 text-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-ring {label
      ? 'font-medium text-foreground'
      : 'text-muted-foreground'}"
  >
    <option value="">{s.allLabels}</option>
    {#each LABEL_OPTIONS as [value, text] (value)}
      <option {value}>{text}</option>
    {/each}
  </select>

  <div class="relative min-w-[11rem] flex-1">
    <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
    <input
      type="search"
      placeholder={s.searchPlaceholder}
      aria-label={s.searchAria}
      bind:value={search}
      oninput={onSearchInput}
      class="w-full rounded-lg border border-border bg-background py-2 pl-9 pr-3 text-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-ring"
    />
  </div>

  <!-- Compact icon actions, grouped: unread toggle · mark all read · refresh. -->
  <div class="flex items-center gap-1">
    <button
      type="button"
      onclick={onToggleUnread}
      aria-pressed={unread}
      title={s.unreadOnly}
      aria-label={s.unreadOnly}
      class="rounded-lg border p-2 transition-colors {unread
        ? 'border-brand-ring bg-brand-muted/60 text-foreground'
        : 'border-border text-muted-foreground hover:text-foreground'}"
    >
      <Mail class="h-4 w-4" />
    </button>
    <button
      type="button"
      onclick={onMarkAllRead}
      disabled={markingAll}
      title={s.markAllRead}
      aria-label={s.markAllRead}
      class="rounded-lg border border-border p-2 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
    >
      <CheckCheck class="h-4 w-4" />
    </button>
    <button
      type="button"
      onclick={onRefresh}
      disabled={refreshing}
      title={s.refresh}
      aria-label={s.refresh}
      class="rounded-lg border border-border p-2 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
    >
      <RefreshCw class="h-4 w-4 {refreshing ? 'animate-spin' : ''}" />
    </button>
  </div>
</div>
