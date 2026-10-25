<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/state';
  import { ApiError, api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { resolve } from '$app/paths';
  import { buddyDock } from '#lib/buddyDock.svelte.js';
  import type { BuddyFeedItem, BuddySimilar, BuddyStatus } from '#lib/types.js';
  import { Button, ConfirmDialog, EntityLogo } from '#lib/ui/index.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import BuddyChatView from './BuddyChatView.svelte';
  import BuddyFeedEntry from './BuddyFeedEntry.svelte';
  import BuddyInviteDialog from './BuddyInviteDialog.svelte';
  import BuddyMessageButton from './BuddyMessageButton.svelte';
  import States from './States.svelte';

  type Tab = 'similar' | 'feed' | 'chats';
  // The notification link /my/buddies?c=<id> names a conversation to open on the Chats tab.
  const linkedParam = Number(page.url.searchParams.get('c'));
  const linkedAtLoad = Number.isInteger(linkedParam) && linkedParam > 0 ? linkedParam : null;
  // "Message" on a member who already has a conversation with the viewer re-aims it here.
  let linkedConversation = $state<number | null>(linkedAtLoad);
  let tab = $state<Tab>(linkedAtLoad ? 'chats' : 'similar');
  let mode = $state<'similar' | 'all'>('similar');
  const status = new AsyncData<BuddyStatus | null>(null);
  const similar = new AsyncData<BuddySimilar[]>([]);
  let feed = $state<BuddyFeedItem[]>([]);
  let cursor = $state<string | null>(null);
  let feedBusy = $state(false);
  let inviteHandle = $state('');
  let inviteOpen = $state(false);
  let membershipError = $state('');
  let confirmLeaveOpen = $state(false);

  onMount(() => { void status.run(() => api.buddyStatus()); });
  // Membership decides whether the messaging dock exists, so it is re-probed after either change.
  async function join() { await status.run(() => api.joinBuddies()); buddyDock.reset(); }
  async function leave() {
    membershipError = '';
    try { await api.leaveBuddies(); buddyDock.reset(); await status.run(() => api.buddyStatus()); }
    catch (err) { membershipError = err instanceof Error ? err.message : 'Could not leave interview buddies.'; }
  }
  async function loadSimilar() { await similar.run(() => api.listBuddySimilar()); }
  async function loadFeed(next = false, requestedMode = mode) {
    if (feedBusy) return;
    feedBusy = true;
    try {
      const page = await api.listBuddyFeed(requestedMode, next ? cursor ?? undefined : undefined);
      if (requestedMode !== mode) return;
      feed = next ? [...feed, ...page.data] : page.data;
      cursor = page.meta.next_cursor;
    } catch (err) {
      if (next && err instanceof ApiError && err.status === 400) {
        cursor = null;
        const firstPage = await api.listBuddyFeed(requestedMode);
        if (requestedMode !== mode) return;
        feed = firstPage.data;
        cursor = firstPage.meta.next_cursor;
      }
    } finally {
      feedBusy = false;
      if (requestedMode !== mode && status.value?.member && tab === 'feed') {
        feed = []; cursor = null;
        void loadFeed(false, mode);
      }
    }
  }
  async function loadConversations() {
    // The viewer's conversations (the shared list the dock reads too), so each member's
    // "Message" on the Similar and Feed tabs knows whether one already exists. Without it
    // every member offers a first message; the server still refuses a second conversation
    // with the same member, so a failed read degrades, never misleads.
    try { await buddyDock.refresh(); } catch { /* keep the last list */ }
  }
  function openInvite(handle: string) { inviteHandle = handle; inviteOpen = true; }
  function openConversation(id: number) { linkedConversation = id; tab = 'chats'; }
  $effect(() => { if (status.value?.member && tab === 'similar') void loadSimilar(); });
  $effect(() => { if (status.value?.member && tab !== 'chats') void loadConversations(); });
  $effect(() => { const member = status.value?.member; const currentTab = tab; const currentMode = mode; if (member && currentTab === 'feed') untrack(() => { feed = []; cursor = null; void loadFeed(false, currentMode); }); });
</script>

{#if status.status === 'loading'}<States state="loading" />
{:else if status.status === 'error'}<States state="error" />
{:else if !status.value?.member}
  <section class="rounded-lg border border-border p-6">
    <h1 class="text-2xl font-semibold">Interview buddies</h1>
    <p class="mt-3 text-sm">Join an anonymous peer network to find people with similar job searches and talk after they accept your invitation.</p>
    <p class="mt-3 text-sm">Other members will see your anonymous card (role, seniority, skills, country), the companies and job titles you apply to, and when you move to the interview or offer stage, including applications you made before joining. They never see your name or employer. You can hide any application, and you can leave any time.</p>
    <Button class="mt-5" variant="primary" onclick={join}>Join interview buddies</Button>
  </section>
{:else}
  <section class="flex flex-col gap-5">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div><h1 class="text-2xl font-semibold">Interview buddies</h1><p class="text-sm text-muted-foreground">Your handle: {status.value.handle}</p></div>
      <Button variant="outline" size="sm" onclick={() => (confirmLeaveOpen = true)}>Leave</Button>
    </header>
    {#if membershipError}<p role="alert" class="text-sm text-destructive">{membershipError}</p>{/if}
    <nav class="flex gap-2 border-b border-border" aria-label="Buddy sections">
      {#each ['similar', 'feed', 'chats'] as item (item)}<button class="border-b-2 px-3 py-2 text-sm capitalize {tab === item ? 'border-brand font-semibold' : 'border-transparent text-muted-foreground'}" aria-pressed={tab === item} onclick={() => tab = item as Tab}>{item}</button>{/each}
    </nav>
    {#if tab === 'similar'}
      {#if similar.status === 'loading'}<States state="loading" />{:else if similar.value.length === 0}<States state="empty" message="No similar members yet." />{:else}
        <ul class="flex flex-col gap-3">{#each similar.value as person (person.handle)}<li class="rounded-lg border border-border p-4">
          <div class="flex items-center justify-between gap-3"><a href={resolve('/my/buddies/[handle]', { handle: person.handle })} class="font-semibold hover:underline">{person.handle}</a><BuddyMessageButton handle={person.handle} conversations={buddyDock.conversations} oninvite={openInvite} onconversation={openConversation} /></div>
          <p class="mt-1 text-sm text-muted-foreground">{[person.card_summary.category, person.card_summary.seniority, person.card_summary.country].filter(Boolean).join(' · ')}</p>
          {#if person.shared_companies.length}<div class="mt-3 flex flex-wrap gap-3">{#each person.shared_companies as company (company.slug)}<span class="flex items-center gap-1.5 text-xs"><EntityLogo name={company.name} src={companyLogoUrl(company.name) ?? undefined} shape="square" size="xs" />{company.name}</span>{/each}</div>{/if}
        </li>{/each}</ul>
      {/if}
    {:else if tab === 'feed'}
      <div class="flex gap-2"><Button variant={mode === 'similar' ? 'primary' : 'outline'} size="sm" onclick={() => mode = 'similar'}>Similar</Button><Button variant={mode === 'all' ? 'primary' : 'outline'} size="sm" onclick={() => mode = 'all'}>All</Button></div>
      {#if feed.length === 0 && !feedBusy}<States state="empty" message="No application updates yet." />{:else}<ul>{#each feed as item (item.id)}<BuddyFeedEntry {item}>{#snippet actions()}<BuddyMessageButton handle={item.handle} conversations={buddyDock.conversations} oninvite={openInvite} onconversation={openConversation} />{/snippet}</BuddyFeedEntry>{/each}</ul>{#if cursor}<Button variant="outline" onclick={() => loadFeed(true)} disabled={feedBusy}>Load more</Button>{/if}{/if}
    {:else}<BuddyChatView initialId={linkedConversation} />{/if}
  </section>
{/if}

<BuddyInviteDialog bind:open={inviteOpen} handle={inviteHandle} onsent={(conversation) => openConversation(conversation.id)} />

<ConfirmDialog
  bind:open={confirmLeaveOpen}
  title="Leave interview buddies?"
  description="You disappear from other members' lists and feed, and every conversation closes. You can join again later with the same handle."
  confirmLabel="Leave"
  onConfirm={leave}
/>
