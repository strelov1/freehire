<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/state';
  import { ApiError, api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import type { BuddyFeedItem, BuddySimilar, BuddyStatus } from '#lib/types.js';
  import { Button, ConfirmDialog, Dialog, EntityLogo } from '#lib/ui/index.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import BuddyChatView from './BuddyChatView.svelte';
  import States from './States.svelte';

  type Tab = 'similar' | 'feed' | 'chats';
  // The notification link /my/buddies?c=<id> names a conversation to open on the Chats tab.
  const linkedParam = Number(page.url.searchParams.get('c'));
  const linkedConversation = Number.isInteger(linkedParam) && linkedParam > 0 ? linkedParam : null;
  let tab = $state<Tab>(linkedConversation ? 'chats' : 'similar');
  let mode = $state<'similar' | 'all'>('similar');
  const status = new AsyncData<BuddyStatus | null>(null);
  const similar = new AsyncData<BuddySimilar[]>([]);
  let feed = $state<BuddyFeedItem[]>([]);
  let cursor = $state<string | null>(null);
  let feedBusy = $state(false);
  let activeHandle = $state('');
  let inviteBody = $state('');
  let inviteError = $state('');
  let membershipError = $state('');
  let confirmLeaveOpen = $state(false);
  let busy = $state(false);
  let selected = $state<BuddySimilar | null>(null);
  let inviteOpen = $state(false);
  let profile = $state<{ handle: string; card: Record<string, unknown>; card_empty: boolean } | null>(null);
  let profileOpen = $state(false);

  onMount(() => { void status.run(() => api.buddyStatus()); });
  async function join() { await status.run(() => api.joinBuddies()); }
  async function leave() {
    membershipError = '';
    try { await api.leaveBuddies(); await status.run(() => api.buddyStatus()); }
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
  function openInvite(person: BuddySimilar) {
    selected = person; activeHandle = person.handle; inviteBody = ''; inviteError = ''; inviteOpen = true;
  }
  async function showProfile(handle: string) { try { profile = await api.getBuddyProfile(handle); profileOpen = true; } catch { profile = null; } }
  async function sendInvite() {
    if (!selected || busy || !inviteBody.trim() || inviteBody.length > 500) return;
    busy = true; inviteError = '';
    try { await api.inviteBuddy(selected.handle, inviteBody.trim()); inviteOpen = false; }
    catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        const retryAt = err.body?.retry_at;
        inviteError = `${err.message}${typeof retryAt === 'string' ? ` Try again after ${new Date(retryAt).toLocaleString()}.` : ''}`;
      } else inviteError = err instanceof Error ? err.message : 'Could not send invitation.';
    } finally { busy = false; }
  }
  $effect(() => { if (status.value?.member && tab === 'similar') void loadSimilar(); });
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
          <div class="flex items-center justify-between gap-3"><button class="font-semibold hover:underline" onclick={() => showProfile(person.handle)}>{person.handle}</button><Button size="sm" variant="primary" onclick={() => openInvite(person)}>Invite</Button></div>
          <p class="mt-1 text-sm text-muted-foreground">{[person.card_summary.category, person.card_summary.seniority, person.card_summary.country].filter(Boolean).join(' · ')}</p>
          {#if person.shared_companies.length}<div class="mt-3 flex flex-wrap gap-3">{#each person.shared_companies as company (company.slug)}<span class="flex items-center gap-1.5 text-xs"><EntityLogo name={company.name} src={companyLogoUrl(company.name) ?? undefined} shape="square" size="xs" />{company.name}</span>{/each}</div>{/if}
        </li>{/each}</ul>
      {/if}
    {:else if tab === 'feed'}
      <div class="flex gap-2"><Button variant={mode === 'similar' ? 'primary' : 'outline'} size="sm" onclick={() => mode = 'similar'}>Similar</Button><Button variant={mode === 'all' ? 'primary' : 'outline'} size="sm" onclick={() => mode = 'all'}>All</Button></div>
      {#if feed.length === 0 && !feedBusy}<States state="empty" message="No application updates yet." />{:else}<ul>{#each feed as item (item.id)}<li class="flex items-center gap-3 border-b border-border py-3 text-sm"><button onclick={() => showProfile(item.handle)} class="font-medium hover:underline">{item.handle}</button><span>{item.type} at</span><EntityLogo name={item.company.name} src={companyLogoUrl(item.company.name) ?? undefined} shape="square" size="xs"/><span class="font-medium">{item.company.name}</span>{#if item.job_title}<span class="text-muted-foreground">· {item.job_title}</span>{/if}<time class="ml-auto text-xs text-muted-foreground">{item.day}</time></li>{/each}</ul>{#if cursor}<Button variant="outline" onclick={() => loadFeed(true)} disabled={feedBusy}>Load more</Button>{/if}{/if}
    {:else}<BuddyChatView initialId={linkedConversation} />{/if}
  </section>
{/if}

{#if profile}
  <Dialog bind:open={profileOpen} title={profile.handle} class="sm:max-w-lg">
    {#if profile.card_empty}<p class="text-sm text-muted-foreground">This member has not added a profile card yet.</p>{:else}
      <div class="flex flex-col gap-3 text-sm">
        {#each Object.entries(profile.card) as [key, value] (key)}
          {#if value && !['handle', 'name', 'email', 'username', 'employer'].includes(key)}
            <div><h2 class="font-medium capitalize">{key.replaceAll('_', ' ')}</h2>{#if Array.isArray(value)}<ul class="flex flex-wrap gap-2">{#each value as item, index (`${key}-${index}`)}<li class="rounded bg-muted px-2 py-1">{typeof item === 'string' ? item : JSON.stringify(item)}</li>{/each}</ul>{:else}<p>{String(value)}</p>{/if}</div>
          {/if}
        {/each}
      </div>
    {/if}
  </Dialog>
{/if}

<Dialog bind:open={inviteOpen} title="Invite {activeHandle}" class="sm:max-w-lg">
  <form class="flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); void sendInvite(); }}>
    <textarea bind:value={inviteBody} maxlength="500" rows="5" class="w-full rounded-md border border-border bg-background p-3 text-sm" aria-label="Invitation message"></textarea>
    <span class="text-right text-xs text-muted-foreground">{inviteBody.length}/500</span>
    {#if inviteError}<p class="text-sm text-destructive">{inviteError}</p>{/if}
    <Button type="submit" variant="primary" disabled={busy || !inviteBody.trim()}>Send invitation</Button>
  </form>
</Dialog>

<ConfirmDialog
  bind:open={confirmLeaveOpen}
  title="Leave interview buddies?"
  description="You disappear from other members' lists and feed, and every conversation closes. You can join again later with the same handle."
  confirmLabel="Leave"
  onConfirm={leave}
/>
