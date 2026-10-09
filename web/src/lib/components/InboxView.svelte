<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import type {
    GmailStatus,
    MailboxStatus,
    InboxSource,
    InboxMessage,
    EmailBody,
  } from '$lib/api';
  import type { EmailLinking } from '$lib/types';
  import { statusLabel, statusLabels } from '$lib/emailStatus';
  import StatusChip from '$lib/components/StatusChip.svelte';
  import { inboxLinkState, type LastUnlinked } from '$lib/inboxLink';
  import { Paginator } from '$lib/paginated.svelte';
  import InboxSettings from './InboxSettings.svelte';
  import ApplicationLinkPicker from './ApplicationLinkPicker.svelte';
  import InfiniteScroll from './InfiniteScroll.svelte';
  import {
    Mail,
    Search,
    RefreshCw,
    ChevronLeft,
    CheckCheck,
    Trash2,
    Inbox,
    Settings,
  } from '@lucide/svelte';
  import { TabStrip, tabStripId } from '$lib/ui';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { timeAgo, errorMessage, must } from '$lib/utils';
  import { avatarInitials, avatarColor } from '$lib/avatar';
  import { messages } from './InboxView.messages';
  import { format, plural, t } from '$lib/i18n/t';

  const PAGE_SIZE = 7;

  const s = $derived(t(messages, locale()));

  let gmail = $state<GmailStatus | null>(null);
  let mailbox = $state<MailboxStatus | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Account switcher: '' = all sources, 'gmail' | 'hosted' = one account.
  let source = $state<InboxSource>('');

  // Search: filters by subject/sender/body server-side, debounced.
  let search = $state('');
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  // Triage filters: unread-only and one classification label ('' = all).
  let unread = $state(false);
  let label = $state('');
  // Mail the classifier judged not to be about an application at all is hidden by default —
  // the inbox is where a person looks for their applications. How much that costs them is
  // shown as a number rather than left to be discovered, because a filter nobody can see is
  // a misclassification nobody can find.
  let includeOther = $state(false);
  let hidden = $state(0);
  // The dropdown offers only signals with a human label (drops 'other' → blank).
  const LABEL_OPTIONS = $derived(Object.entries(statusLabels(locale())).filter(([, l]) => l !== ''));
  const filterActive = $derived(unread || label !== '' || search !== '');

  // The mail list pages over the inbox endpoint with the shared Paginator; the
  // fetch closure reads the live filters (search, account, unread, label) at
  // call time, so a filter change just re-fetches the first page.
  const pager = new Paginator<InboxMessage>(
    async (limit, offset) => {
      const page = await api.getInbox({
        q: search,
        limit,
        offset,
        source,
        unread,
        status: label,
        includeOther,
      });
      hidden = page.hidden;
      return page;
    },
    { limit: PAGE_SIZE },
  );

  let refreshing = $state(false);
  let markingAll = $state(false);

  // The message just soft-deleted, for a one-click Undo (per-email, like unlink).
  let lastDeleted = $state<{ id: number; subject: string } | null>(null);

  // Which pane: the mail list ('inbox') or the account setup ('settings'). Both are
  // local state rather than routes, so the strip's tabs are buttons — see TabStrip.
  let tab = $state<'inbox' | 'settings'>('inbox');
  const PANEL_ID = 'inbox-panel';
  const TABS = $derived([
    { id: 'inbox', label: s.tabs.inbox, icon: Inbox },
    { id: 'settings', label: s.tabs.settings, icon: Settings },
  ] as const);

  // The selected message and its loaded body (reading pane).
  let selectedId = $state<number | null>(null);
  let selected = $state<EmailBody | null>(null);
  let bodyLoading = $state(false);

  // Manual linking: the caller's applications for the picker (lazy-loaded once),
  // and the application an email was just unlinked from, for a one-click Undo.
  let trackedApps = $state<{ slug: string; company: string; title?: string }[]>([]);
  let trackedLoaded = $state(false);
  let trackedLoading = $state(false);
  let lastUnlinked = $state<LastUnlinked | null>(null);

  const hasGmail = $derived(!!gmail?.connected);
  const hasMailbox = $derived(!!mailbox?.address);
  // Mail pushed by the caller's own agent harness has no connection to report —
  // it simply exists. So presence is the signal: if the inbox holds anything, the
  // user has a source, whatever set it up.
  let hasPushedMail = $state(false);
  const hasAnySource = $derived(hasGmail || hasMailbox || hasPushedMail);
  // The account switcher only lists sources the caller actually has, and is only
  // worth showing when there is more than one to switch between.
  const presentSources = $derived(
    [
      { value: 'gmail' as InboxSource, label: 'Gmail', present: hasGmail },
      { value: 'hosted' as InboxSource, label: s.sources.mailbox, present: hasMailbox },
      { value: 'external' as InboxSource, label: s.sources.pushed, present: hasPushedMail },
    ].filter((opt) => opt.present),
  );
  const sourceOptions = $derived([{ value: '' as InboxSource, label: s.sources.all }, ...presentSources]);

  // A message addressed from outside the inbox — the tracking calendar links each
  // mail-derived event to the message behind it. Opening it here marks it read, which is
  // correct: read_at means a human saw the message, and this arrival is a person clicking
  // through to it. What must never mark mail read is a view that merely lists events, and
  // that is why the calendar reaches the subject through a join and links here instead of
  // fetching bodies itself.
  function openAddressedMessage() {
    const id = Number(page.url.searchParams.get('message'));
    if (!Number.isInteger(id) || id <= 0) return;
    void openMessage(id);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- shallow same-page URL clean-up to the current pathname; nothing to resolve
    replaceState(page.url.pathname, {});
  }

  onMount(() => {
    // After load(), not before: fetchFirstPage replaces pager.items wholesale, and
    // openMessage patches the opened row to read INSIDE that list. Racing the two leaves
    // the message the reader just opened still showing as unread.
    void load().then(openAddressedMessage);
  });

  async function load() {
    loading = true;
    error = null;
    try {
      const [gmailStatus, mailboxStatus, pushed] = await Promise.all([
        api.gmailStatus(),
        api.mailboxStatus(),
        // One cheap probe for harness-pushed mail, which reports no connection.
        api.getInbox({ source: 'external', limit: 1 }),
      ]);
      gmail = gmailStatus;
      mailbox = mailboxStatus;
      hasPushedMail = (pushed.total ?? 0) > 0;
      if (hasAnySource) await fetchFirstPage(s.errors.loadInbox);
      else tab = 'settings'; // nothing to read yet — land on setup
    } catch (e) {
      error = errorMessage(e, s.errors.loadInbox);
    } finally {
      loading = false;
    }
  }

  // Re-fetch the first page for the current filters, replacing the list. The
  // Paginator swallows fetch errors into its status, so rethrow with the
  // caller's message to surface it like the old manual paging did. `reloading`
  // keeps the infinite-scroll sentinel from paging while a reload is in flight.
  let reloading = $state(false);
  async function fetchFirstPage(failMessage: string) {
    reloading = true;
    try {
      await pager.start();
      if (pager.status === 'error') throw new Error(failMessage);
    } finally {
      reloading = false;
    }
  }

  // Reload the first page; clears the reading pane.
  async function reloadList() {
    selectedId = null;
    selected = null;
    try {
      await fetchFirstPage(s.errors.loadInbox);
    } catch (e) {
      error = errorMessage(e, s.errors.loadInbox);
    }
  }

  // Toolbar refresh — re-fetch the first page, keeping the open message.
  async function refreshInbox() {
    if (refreshing) return;
    refreshing = true;
    error = null;
    try {
      await fetchFirstPage(s.errors.refresh);
    } catch (e) {
      error = errorMessage(e, s.errors.refresh);
    } finally {
      refreshing = false;
    }
  }

  // Infinite scroll: the list is a fixed-height scroll pane (the page itself does
  // not grow); the bottom sentinel pulls the next page in place. A short first
  // page in a tall pane leaves the sentinel visible, so it keeps firing until
  // the list overflows its pane or the mailbox is exhausted.
  async function loadMore() {
    await pager.loadMore();
    if (pager.loadMoreError) error = s.errors.loadMore;
  }

  function onSearchInput() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(reloadList, 250);
  }

  async function setSource(src: InboxSource) {
    if (source === src) return;
    source = src;
    await reloadList();
  }

  async function toggleUnread() {
    unread = !unread;
    await reloadList();
  }

  // Mark every unread message matching the active filters as read. When the unread
  // view is active it should now be empty, so reload; otherwise flip the visible
  // rows optimistically so the dots clear without a refetch.
  async function markAllRead() {
    if (markingAll) return;
    markingAll = true;
    error = null;
    try {
      await api.markAllRead(source, label, search);
      if (unread) {
        await reloadList();
      } else {
        pager.items = pager.items.map((m) => ({ ...m, read: true }));
        if (selected) selected = { ...selected, read: true };
      }
    } catch (e) {
      error = errorMessage(e, s.errors.markAllRead);
      await reloadList();
    } finally {
      markingAll = false;
    }
  }

  // Soft-delete the open message: drop it from the list optimistically, clear the
  // reading pane, and remember it for a one-click Undo (per-email, like unlink).
  async function deleteSelected() {
    if (!selected) return;
    const id = selected.id;
    const subject = selected.subject;
    const prev = pager.items;
    pager.items = pager.items.filter((m) => m.id !== id);
    pager.total = Math.max(0, pager.total - 1);
    selectedId = null;
    selected = null;
    try {
      await api.deleteEmail(id);
      lastDeleted = { id, subject };
    } catch (e) {
      pager.items = prev; // put the row back
      pager.total += 1;
      error = errorMessage(e, s.errors.deleteMessage);
    }
  }

  // Undo the last delete: restore the message and reload so it lands in order.
  async function undoDelete() {
    if (!lastDeleted) return;
    const { id } = lastDeleted;
    lastDeleted = null;
    try {
      await api.restoreEmail(id);
      await reloadList();
    } catch (e) {
      error = errorMessage(e, s.errors.restore);
    }
  }

  // Mobile master-detail: clear the selection to return from the reading pane to
  // the list (on md+ both panes are always visible, so this only matters below md).
  function backToList() {
    selectedId = null;
    selected = null;
  }

  async function openMessage(id: number) {
    if (lastUnlinked && lastUnlinked.id !== id) lastUnlinked = null; // Undo is per-email
    if (lastDeleted && lastDeleted.id !== id) lastDeleted = null;
    selectedId = id;
    selected = null;
    bodyLoading = true;
    try {
      selected = await api.getEmail(id);
      // Reflect the just-opened message as read in the list without a refetch.
      pager.items = pager.items.map((m) => (m.id === id ? { ...m, read: true } : m));
      // An email with no link and no suggestion can be linked by hand — make sure
      // the picker has the caller's applications ready.
      if (!selected.linked_slug && !selected.suggested_slug) void ensureTrackedApps();
    } catch (e) {
      error = errorMessage(e, s.errors.loadMessage);
    } finally {
      bodyLoading = false;
    }
  }

  // Load the caller's applications for the link picker once, on first need.
  async function ensureTrackedApps() {
    if (trackedLoaded || trackedLoading) return;
    trackedLoading = true;
    try {
      const res = await api.listMyJobs('applied', 100, 0);
      // Only postings-backed applications can be offered as a link target: linking
      // mail names a job slug, and an application whose posting was pruned has none.
      trackedApps = res.items
        .filter((m) => m.job)
        .map((m) => {
          const job = must(m.job, 'm.job');
          return { slug: job.public_slug, company: job.company, title: job.title };
        });
      trackedLoaded = true;
    } catch {
      // Leave the list empty; the picker shows its empty state.
    } finally {
      trackedLoading = false;
    }
  }

  // --- Email → application linking ---

  // Copy the link overlay from a refreshed body back onto both the open message
  // and its list row, so the chip/badge update without a refetch.
  function applyLinkUpdate(updated: EmailBody) {
    selected = updated;
    const overlay: EmailLinking = {
      status_signal: updated.status_signal,
      link_source: updated.link_source,
      linked_slug: updated.linked_slug,
      linked_company: updated.linked_company,
      suggested_slug: updated.suggested_slug,
      suggested_company: updated.suggested_company,
    };
    pager.items = pager.items.map((m) => (m.id === updated.id ? { ...m, ...overlay } : m));
  }

  async function confirmLink() {
    if (!selected) return;
    try {
      applyLinkUpdate(await api.confirmEmailLink(selected.id));
    } catch (e) {
      error = errorMessage(e, s.errors.link);
    }
  }

  async function rejectLink() {
    if (!selected) return;
    try {
      applyLinkUpdate(await api.rejectEmailLink(selected.id));
      // Dismissing the suggestion drops the email into the manual picker — load its data.
      void ensureTrackedApps();
    } catch (e) {
      error = errorMessage(e, s.errors.dismiss);
    }
  }

  async function unlink() {
    if (!selected) return;
    const prevSlug = selected.linked_slug;
    const prevCompany = selected.linked_company;
    try {
      applyLinkUpdate(await api.unlinkEmail(selected.id));
      // Remember what it was linked to so the row can offer a one-click Undo.
      if (prevSlug) lastUnlinked = { id: selected.id, slug: prevSlug, company: prevCompany };
      // The row now also offers the picker (to link elsewhere) — make sure it has data.
      void ensureTrackedApps();
    } catch (e) {
      error = errorMessage(e, s.errors.unlink);
    }
  }

  // Manually link the open email to a chosen application (also used to relink).
  async function linkTo(slug: string) {
    if (!selected) return;
    try {
      applyLinkUpdate(await api.linkEmail(selected.id, slug));
      lastUnlinked = null;
    } catch (e) {
      error = errorMessage(e, s.errors.link);
    }
  }

  // Undo the last unlink: relink the email to the application it was unlinked from.
  async function undoUnlink() {
    if (!selected || !lastUnlinked) return;
    await linkTo(lastUnlinked.slug);
  }

  // --- Gmail source ---

  // Deep link to a Gmail message in Gmail's web UI (the Gmail API id is the URL id).
  const gmailUrl = (externalId: string) =>
    `https://mail.google.com/mail/?authuser=${encodeURIComponent(gmail?.email ?? '')}#all/${externalId}`;

  // A source was added or removed in the Settings pane. A filter pointing at an
  // account that no longer exists would render an empty list that looks like "no
  // mail" rather than "no such account", so clear it before reloading.
  async function onSourceChanged(removed: InboxSource | null) {
    if (removed && source === removed) source = '';
    await refresh();
  }

  // Refresh the listing after a source is added/removed; empties it when none left.
  async function refresh() {
    if (!hasAnySource) {
      pager.seed({ items: [], total: 0, hasMore: false });
      selectedId = null;
      selected = null;
      return;
    }
    await reloadList();
  }
</script>

{#if loading}
  <p class="py-12 text-center text-sm text-muted-foreground">{s.loading}</p>
{:else if error}
  <p class="text-sm text-destructive">{error}</p>
{:else}
  <div class="flex flex-col gap-4">
    <TabStrip
      tabs={TABS}
      active={tab}
      onSelect={(id) => (tab = id)}
      label={s.tabStripLabel}
      panelId={PANEL_ID}
    />

    <div
      id={PANEL_ID}
      role="tabpanel"
      aria-labelledby={tabStripId(PANEL_ID, tab)}
      class="flex flex-col gap-4"
    >
      {#if tab === 'settings'}
        <InboxSettings {gmail} bind:mailbox onSourceChanged={onSourceChanged} onError={(m) => (error = m)} />
      {:else if !hasAnySource}
        <p class="py-8 text-center text-sm text-muted-foreground">
          {s.noSource.lead}
          <button type="button" class="font-medium text-primary hover:underline" onclick={() => (tab = 'settings')}>{s.noSource.link}</button>.
        </p>
      {:else}
        <!-- Toolbar: account switcher + label filter + search on the left; a compact
             icon cluster (unread filter, mark-all-read, refresh) on the right. -->
        <div class="flex flex-wrap items-center gap-2">
          {#if presentSources.length > 1}
            <div class="flex gap-1 rounded-lg border border-border p-1 text-sm">
              {#each sourceOptions as opt (opt.value)}
                <button
                  type="button"
                  onclick={() => setSource(opt.value as InboxSource)}
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
            onchange={reloadList}
            aria-label={s.filterByLabelAria}
            class="rounded-lg border border-border bg-background py-2 pl-3 pr-8 text-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand-ring/40 {label
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
              bind:value={search}
              oninput={onSearchInput}
              class="w-full rounded-lg border border-border bg-background py-2 pl-9 pr-3 text-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand-ring/40"
            />
          </div>

          <!-- Compact icon actions, grouped: unread toggle · mark all read · refresh. -->
          <div class="flex items-center gap-1">
            <button
              type="button"
              onclick={toggleUnread}
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
              onclick={markAllRead}
              disabled={markingAll}
              title={s.markAllRead}
              aria-label={s.markAllRead}
              class="rounded-lg border border-border p-2 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
            >
              <CheckCheck class="h-4 w-4" />
            </button>
            <button
              type="button"
              onclick={refreshInbox}
              disabled={refreshing}
              title={s.refresh}
              aria-label={s.refresh}
              class="rounded-lg border border-border p-2 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
            >
              <RefreshCw class="h-4 w-4 {refreshing ? 'animate-spin' : ''}" />
            </button>
          </div>
        </div>

        {#if lastDeleted}
          <div class="flex items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
            {format(s.deletedToast, { subject: lastDeleted.subject || s.noSubject })}
            <span aria-hidden="true">·</span>
            <button type="button" onclick={undoDelete} class="font-medium text-brand-strong hover:underline">{s.undo}</button>
          </div>
        {/if}

        <!-- The number the default cost them, where they are already looking. It appears only
             when something was hidden, so a clean mailbox says nothing at all. -->
        {#if hidden > 0 && !includeOther}
          <p class="pb-3 text-sm text-muted-foreground">
            {format(plural(locale(), hidden, s.hiddenCount), { count: String(hidden) })}
            <button
              type="button"
              class="font-medium text-brand-strong underline-offset-2 hover:underline"
              onclick={() => {
                includeOther = true;
                void fetchFirstPage(s.errors.loadHidden);
              }}
            >
              {s.show}
            </button>
          </p>
        {:else if includeOther}
          <p class="pb-3 text-sm text-muted-foreground">
            {s.showingHidden}
            <button
              type="button"
              class="font-medium text-brand-strong underline-offset-2 hover:underline"
              onclick={() => {
                includeOther = false;
                void fetchFirstPage(s.errors.reload);
              }}
            >
              {s.hide}
            </button>
          </p>
        {/if}

        {#if pager.items.length === 0}
          <p class="py-12 text-center text-sm text-muted-foreground">
            {filterActive ? s.emptyFiltered : s.emptyNone}
          </p>
        {:else}
          <!-- Fixed-height two-pane so the page itself never scrolls: each pane scrolls
               internally, and the list infinite-loads in place. On md+ side by side; below
               md a master-detail (open a message → the reading pane replaces the list). -->
          <div class="grid h-[calc(100dvh-12rem)] min-h-[26rem] gap-5 md:grid-cols-[minmax(0,19rem)_1fr]">
            <div
              class="min-h-0 flex-col gap-1 overflow-y-auto pr-1 {selectedId === null ? 'flex' : 'hidden md:flex'}"
            >
              <ul class="flex flex-col gap-1">
                {#each pager.items as m, i (m.id)}
                  <li class="row-in" style="animation-delay: {Math.min(i, 14) * 15}ms">
                    <button
                      type="button"
                      onclick={() => openMessage(m.id)}
                      aria-current={selectedId === m.id}
                      class="flex w-full items-start gap-3 rounded-xl border p-3 text-left transition-colors {selectedId === m.id
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
                            <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-brand" aria-label={s.unreadDotAria}></span>
                          {/if}
                          <span class="min-w-0 flex-1 truncate text-sm {m.read ? 'font-medium text-foreground/90' : 'font-semibold text-foreground'}">
                            {m.from_name || m.from_addr}
                          </span>
                          <span class="shrink-0 text-[11px] text-muted-foreground">{timeAgo(m.received_at, locale())}</span>
                        </div>
                        <div class="mt-0.5 truncate text-sm {m.read ? 'text-muted-foreground' : 'text-foreground'}">
                          {m.subject || s.noSubject}
                        </div>
                        {#if m.snippet}
                          <div class="mt-0.5 truncate text-xs text-muted-foreground/80">{m.snippet}</div>
                        {/if}
                        {#if statusLabel(m.status_signal, locale()) || m.linked_slug}
                          <div class="mt-1 flex items-center gap-1">
                            <StatusChip signal={m.status_signal} class="text-[10px] leading-4" />
                            {#if m.linked_slug}
                              <span class="truncate text-[10px] text-muted-foreground/70">· {m.linked_company}</span>
                            {:else if m.suggested_slug}
                              <span class="text-[10px] text-brand-strong">· {s.suggestedTag}</span>
                            {/if}
                          </div>
                        {/if}
                      </div>
                    </button>
                  </li>
                {/each}
              </ul>

              {#if pager.hasMore}
                <!-- Scroll-to-bottom auto-load (inside the list's own scroll pane). -->
                <InfiniteScroll
                  onLoad={loadMore}
                  enabled={!pager.loadingMore && !pager.loadMoreError && !reloading}
                />
              {/if}
              {#if pager.loadingMore}
                <p class="py-3 text-center text-xs text-muted-foreground">{s.loading}</p>
              {/if}
            </div>

            <!-- Reading pane — borderless, flush, fills the pane height and scrolls
                 internally. On mobile it replaces the list once a message is open. -->
            <div class="flex min-h-0 flex-col {selectedId === null ? 'hidden md:flex' : 'flex'}">
              <button
                type="button"
                onclick={backToList}
                class="mb-3 -ml-1 flex shrink-0 items-center gap-1 rounded-md px-1 py-1 text-sm text-muted-foreground hover:text-foreground md:hidden"
              >
                <ChevronLeft class="h-4 w-4" /> {s.tabs.inbox}
              </button>
              {#if bodyLoading}
                <p class="py-16 text-center text-sm text-muted-foreground">{s.loading}</p>
              {:else if !selected}
                <div class="flex h-full flex-col items-center justify-center gap-2 py-16 text-center">
                  <Mail class="h-7 w-7 text-muted-foreground/50" />
                  <p class="text-sm text-muted-foreground">{s.selectMessage}</p>
                </div>
              {:else}
                {@const msg = selected}
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
                      <span class="ml-auto shrink-0">{timeAgo(msg.received_at, locale())}</span>
                    </div>
                  </div>
                  <button
                    type="button"
                    onclick={deleteSelected}
                    title={s.deleteTitle}
                    aria-label={s.deleteMessageAria}
                    class="shrink-0 rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                  >
                    <Trash2 class="h-4 w-4" />
                  </button>
                </div>

                {@const linkState = inboxLinkState(msg, lastUnlinked)}
                <div class="mt-3 flex shrink-0 flex-wrap items-center gap-2">
                  <StatusChip signal={msg.status_signal} />
                  {#if linkState === 'linked' && msg.linked_slug}
                    <a
                      href={resolve('/my/tracking/[id]', { id: msg.linked_slug })}
                      class="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-0.5 text-xs text-muted-foreground transition-colors hover:border-brand-ring hover:text-foreground"
                    >
                      {format(s.linkedTo, { company: msg.linked_company || s.linkedFallback })} ↗
                    </a>
                    <button type="button" onclick={unlink} class="text-xs text-muted-foreground hover:text-destructive">{s.unlink}</button>
                  {:else if linkState === 'suggested'}
                    <span class="inline-flex items-center gap-2 rounded-full border border-brand-ring/50 bg-brand-muted/40 px-2.5 py-0.5 text-xs">
                      {s.looksLike} <span class="font-medium">{msg.suggested_company || s.suggestedFallback}</span>
                      <button type="button" onclick={confirmLink} class="font-medium text-brand-strong hover:underline">{s.link}</button>
                      <span aria-hidden="true">·</span>
                      <button type="button" onclick={rejectLink} class="text-muted-foreground hover:text-foreground">{s.notThis}</button>
                    </span>
                  {:else}
                    {#if linkState === 'undo'}
                      <span class="inline-flex items-center gap-2 text-xs text-muted-foreground">
                        {s.unlinked}
                        <span aria-hidden="true">·</span>
                        <button type="button" onclick={undoUnlink} class="font-medium text-brand-strong hover:underline">{s.undo}</button>
                      </span>
                    {/if}
                    <ApplicationLinkPicker applications={trackedApps} loading={trackedLoading} onpick={linkTo} />
                  {/if}
                </div>

                {#if msg.source === 'gmail'}
                  <div class="mt-2 flex shrink-0 justify-end">
                    <!-- eslint-disable svelte/no-navigation-without-resolve -- external Gmail deep-link, not an internal route -->
                    <a
                      href={gmailUrl(msg.external_id)}
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
                  <pre class="min-h-0 flex-1 overflow-y-auto whitespace-pre-wrap font-sans text-sm leading-relaxed">{msg.body_text}</pre>
                {/if}
              {/if}
            </div>
          </div>
        {/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  @keyframes row-in {
    from {
      opacity: 0;
      transform: translateY(3px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
  .row-in {
    animation: row-in 0.22s ease both;
  }
  @media (prefers-reduced-motion: reduce) {
    .row-in {
      animation: none;
    }
  }
</style>
