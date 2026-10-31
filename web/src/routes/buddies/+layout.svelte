<script lang="ts">
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';
  import { House, LogOut, MessagesSquare, UsersRound, Video } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import { totalUnread } from '#lib/messagingDock.js';
  import type { BuddyStatus } from '#lib/types.js';
  import { Button, ConfirmDialog, cn } from '#lib/ui/index.js';
  import States from '#lib/components/States.svelte';

  // The interview buddies space: its own navigation on the left and the page beside it,
  // full width because the /my shell's sidebar left no room for a feed and a rail.
  // Membership is read here once, so a page never renders for an account that has not
  // joined — the join screen takes its place on every address in the space.
  let { children }: { children: Snippet } = $props();

  const status = new AsyncData<BuddyStatus | null>(null);
  let membershipError = $state('');
  let confirmLeaveOpen = $state(false);

  onMount(() => { void status.run(() => api.buddyStatus()); });

  // Joining or leaving changes which conversations exist, so the shared list is re-read.
  async function join() { await status.run(() => api.joinBuddies()); messagingDock.reset(); }
  async function leave() {
    membershipError = '';
    try { await api.leaveBuddies(); messagingDock.reset(); await status.run(() => api.buddyStatus()); }
    catch (err) { membershipError = err instanceof Error ? err.message : 'Could not leave interview buddies.'; }
  }

  // The Chats badge reads the same list the dock does.
  $effect(() => { if (status.value?.member) void messagingDock.refresh().catch(() => {}); });
  const unread = $derived(totalUnread(messagingDock.conversations));

  const nav = $derived([
    { route: '/buddies', label: 'Feed', icon: House, badge: 0 },
    { route: '/buddies/network', label: 'Network', icon: UsersRound, badge: 0 },
    { route: '/buddies/chats', label: 'Chats', icon: MessagesSquare, badge: unread },
    { route: '/buddies/mocks', label: 'Mock interviews', icon: Video, badge: 0 },
  ] as const);
</script>

<svelte:head>
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-4 lg:py-6">
  {#if status.status === 'loading'}<States state="loading" />
  {:else if status.status === 'error'}<States state="error" onRetry={() => void status.run(() => api.buddyStatus())} />
  {:else if !status.value?.member}
    <section class="mx-auto max-w-2xl rounded-xl border border-border bg-card p-6">
      <h1 class="text-2xl font-semibold">Interview buddies</h1>
      <p class="mt-3 text-sm">Join an anonymous peer network to find people with similar job searches and talk after they accept your invitation.</p>
      <p class="mt-3 text-sm">Other members will see your anonymous card (role, seniority, skills, country), the companies and job titles you apply to, and when you move to the interview or offer stage, including applications you made before joining. They never see your name or employer. You can hide any application, and you can leave any time.</p>
      <Button class="mt-5" variant="primary" onclick={join}>Join interview buddies</Button>
    </section>
  {:else}
    <div class="flex flex-col gap-4 lg:flex-row lg:gap-6">
      <aside class="lg:sticky lg:top-20 lg:w-52 lg:shrink-0 lg:self-start">
        <nav aria-label="Interview buddies" class="flex gap-1 overflow-x-auto rounded-xl border border-border bg-card p-1.5 lg:flex-col">
          {#each nav as item (item.route)}
            {@const Icon = item.icon}
            {@const current = page.url.pathname === resolve(item.route)}
            <a
              href={resolve(item.route)}
              aria-current={current ? 'page' : undefined}
              class={cn(
                'flex shrink-0 items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                current ? 'bg-brand-muted text-brand-strong' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              <Icon class="size-4" aria-hidden="true" />
              {item.label}
              {#if item.badge > 0}
                <span class="ml-auto rounded-full bg-brand px-1.5 text-xs font-semibold tabular-nums text-brand-foreground">
                  {item.badge}<span class="sr-only"> unread</span>
                </span>
              {/if}
            </a>
          {/each}
        </nav>
        <div class="mt-3 px-1 text-xs text-muted-foreground">
          <p>You are <span class="font-medium text-foreground">{status.value.handle}</span>. Members see your role, country and companies, never your name or employer.</p>
          <button type="button" class="mt-3 inline-flex items-center gap-1.5 font-medium hover:text-foreground" onclick={() => (confirmLeaveOpen = true)}>
            <LogOut class="size-3.5" aria-hidden="true" />Leave interview buddies
          </button>
          {#if membershipError}<p role="alert" class="mt-2 text-destructive">{membershipError}</p>{/if}
        </div>
      </aside>
      <div class="min-w-0 flex-1">{@render children()}</div>
    </div>
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmLeaveOpen}
  title="Leave interview buddies?"
  description="You disappear from other members' lists and feed, every conversation closes and your upcoming mock interviews are cancelled. You can join again later with the same handle."
  confirmLabel="Leave"
  onConfirm={leave}
/>
