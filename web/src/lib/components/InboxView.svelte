<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { GmailStatus, InboxGroup, InboxMessage, EmailBody } from '$lib/api';
  import { Badge, Button } from '$lib/ui';
  import { Mail } from '@lucide/svelte';
  import { timeAgo } from '$lib/utils';

  const PAGE_SIZE = 20;

  let status = $state<GmailStatus | null>(null);
  let groups = $state<InboxGroup[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Search: filters groups by message subject/sender/body server-side, debounced.
  let search = $state('');
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  let syncing = $state(false);

  // Expanded group's messages, cached by key; and the opened message body.
  let expanded = $state<string | null>(null);
  let threads = $state<Record<string, InboxMessage[]>>({});
  let openMsgId = $state<number | null>(null);
  let bodies = $state<Record<number, EmailBody>>({});

  onMount(load);

  async function load() {
    loading = true;
    error = null;
    try {
      status = await api.gmailStatus();
      if (status.connected) {
        await fetchFirstPage();
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load the inbox.';
    } finally {
      loading = false;
    }
  }

  // Load the first page of groups for the current search term.
  async function fetchFirstPage() {
    const res = await api.getInbox(search, PAGE_SIZE, 0);
    groups = res.groups;
    total = res.total;
  }

  // Reload the first page for the current search term; collapses any open group.
  async function reloadGroups() {
    expanded = null;
    openMsgId = null;
    try {
      await fetchFirstPage();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Search failed.';
    }
  }

  // Append the next page of groups.
  async function loadMore() {
    try {
      const res = await api.getInbox(search, PAGE_SIZE, groups.length);
      groups = [...groups, ...res.groups];
      total = res.total;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load more.';
    }
  }

  function onSearchInput() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(reloadGroups, 250);
  }

  // Trigger a background sync, then poll the inbox so new mail appears as it lands.
  async function sync() {
    if (syncing) return;
    syncing = true;
    error = null;
    try {
      await api.syncGmail();
      for (let i = 0; i < 8; i++) {
        await new Promise((r) => setTimeout(r, 2500));
        await fetchFirstPage();
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Sync failed.';
    } finally {
      syncing = false;
    }
  }

  function connect() {
    // Top-level navigation: the OAuth flow redirects the browser to Google.
    window.location.href = '/api/v1/me/gmail/connect';
  }

  // Deep link to the message in Gmail's web UI (the Gmail API id is the URL id).
  // authuser targets the connected account, so it opens in the right mailbox even
  // when the user is signed into several Google accounts.
  const gmailUrl = (gmailMsgId: string) =>
    `https://mail.google.com/mail/?authuser=${encodeURIComponent(status?.email ?? '')}#all/${gmailMsgId}`;

  async function disconnect() {
    if (!confirm('Disconnect Gmail and remove all synced mail?')) return;
    try {
      await api.disconnectGmail();
      status = { connected: false };
      groups = [];
      expanded = null;
      openMsgId = null;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to disconnect.';
    }
  }

  async function toggleGroup(key: string) {
    if (expanded === key) {
      expanded = null;
      return;
    }
    expanded = key;
    openMsgId = null;
    if (!threads[key]) {
      try {
        threads = { ...threads, [key]: await api.getInboxGroup(key) };
      } catch (e) {
        error = e instanceof Error ? e.message : 'Failed to load the thread.';
      }
    }
  }

  async function openMessage(id: number) {
    if (openMsgId === id) {
      openMsgId = null;
      return;
    }
    openMsgId = id;
    if (!bodies[id]) {
      try {
        bodies = { ...bodies, [id]: await api.getEmail(id) };
      } catch (e) {
        error = e instanceof Error ? e.message : 'Failed to load the message.';
      }
    }
  }
</script>

{#if loading}
  <p class="py-12 text-center text-sm text-muted-foreground">Loading…</p>
{:else if error}
  <p class="text-sm text-destructive">{error}</p>
{:else if !status?.connected}
  <div class="rounded-xl border border-border bg-card p-8 text-center">
    <h2 class="text-lg font-semibold tracking-tight">Connect your Gmail</h2>
    <p class="mx-auto mt-1 max-w-md text-sm text-muted-foreground">
      See every reply to your job applications in one place — confirmations,
      interview invites, and rejections — automatically grouped by subject.
    </p>
    {#if status?.available}
      <Button variant="primary" class="mt-4" onclick={connect}>
        Connect Gmail
        <Mail class="h-4 w-4" />
      </Button>
    {:else}
      <p class="mt-4 text-sm text-muted-foreground">Gmail connection isn't available yet.</p>
    {/if}
  </div>
{:else}
  <div class="flex flex-col gap-4">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <span class="text-sm text-muted-foreground">
        Connected: <span class="font-medium text-foreground">{status.email}</span>
        {#if status.status === 'needs_reconsent'}
          <Badge variant="outline" class="ml-2 border-destructive/40 text-destructive">Reconnect needed</Badge>
        {/if}
      </span>
      <div class="flex gap-2">
        {#if status.status === 'needs_reconsent'}
          <Button variant="secondary" onclick={connect}>Reconnect</Button>
        {/if}
        <Button variant="secondary" disabled={syncing} onclick={sync}>
          {syncing ? 'Syncing…' : 'Sync'}
        </Button>
        <Button variant="outline" onclick={disconnect}>Disconnect</Button>
      </div>
    </div>

    <input
      type="search"
      placeholder="Search subject, sender, or body…"
      bind:value={search}
      oninput={onSearchInput}
      class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
    />

    {#if groups.length === 0}
      <p class="py-12 text-center text-sm text-muted-foreground">
        {search ? 'No mail matches your search.' : 'No ATS mail yet — it appears here after the next sync.'}
      </p>
    {:else}
      <ul class="flex flex-col gap-2">
        {#each groups as g (g.key)}
          <li class="overflow-hidden rounded-xl border border-border bg-card transition hover:border-brand">
            <button
              type="button"
              onclick={() => toggleGroup(g.key)}
              class="flex w-full items-center gap-3 p-4 text-left hover:bg-accent"
            >
              <div class="min-w-0 flex-1">
                <div class="truncate font-medium">{g.subject || '(no subject)'}</div>
                <div class="truncate text-xs text-muted-foreground">{g.senders.join(', ')}</div>
              </div>
              <span class="shrink-0 text-xs text-muted-foreground">{timeAgo(g.latest_received)}</span>
              <Badge variant="secondary">{g.message_count}</Badge>
            </button>

            {#if expanded === g.key}
              <div class="border-t border-border p-3">
                {#if !threads[g.key]}
                  <p class="text-sm text-muted-foreground">Loading…</p>
                {:else}
                  <ul class="flex flex-col gap-2">
                    {#each threads[g.key] as m (m.id)}
                      <li class="overflow-hidden rounded-lg border border-border">
                        <button
                          type="button"
                          onclick={() => openMessage(m.id)}
                          class="flex w-full flex-col gap-0.5 p-3 text-left hover:bg-accent"
                        >
                          <div class="flex items-center gap-2">
                            <span class="min-w-0 truncate text-sm font-medium">{m.subject}</span>
                            <span class="ml-auto shrink-0 text-xs text-muted-foreground">{timeAgo(m.received_at)}</span>
                          </div>
                          <div class="truncate text-xs text-muted-foreground">{m.from_name || m.from_addr}</div>
                        </button>
                        {#if openMsgId === m.id}
                          <div class="border-t border-border p-3">
                            {#if !bodies[m.id]}
                              <p class="text-sm text-muted-foreground">Loading…</p>
                            {:else}
                              {@const body = bodies[m.id]}
                              <div class="mb-2 flex justify-end">
                                <a
                                  href={gmailUrl(body?.gmail_msg_id ?? '')}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  class="text-xs font-medium text-primary hover:underline"
                                >
                                  Open in Gmail ↗
                                </a>
                              </div>
                              {#if body?.body_html}
                                <!-- Untrusted sender HTML isolated in a sandboxed iframe (no scripts/forms/navigation). -->
                                <iframe
                                  title="Message body"
                                  sandbox=""
                                  srcdoc={body.body_html}
                                  class="h-[24rem] w-full rounded-md border border-border bg-white"
                                ></iframe>
                              {:else}
                                <pre class="whitespace-pre-wrap font-sans text-sm">{body?.body_text}</pre>
                              {/if}
                            {/if}
                          </div>
                        {/if}
                      </li>
                    {/each}
                  </ul>
                {/if}
              </div>
            {/if}
          </li>
        {/each}
      </ul>

      {#if groups.length < total}
        <div class="flex justify-center pt-1">
          <Button variant="outline" onclick={loadMore}>
            Load more ({groups.length} of {total})
          </Button>
        </div>
      {/if}
    {/if}
  </div>
{/if}
