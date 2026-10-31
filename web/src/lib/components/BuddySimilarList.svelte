<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { buddiesByCompany } from '#lib/buddyNetwork.js';
  import { buddySharedCompanies } from '#lib/buddyPresent.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import type { BuddySimilar } from '#lib/types.js';
  import { LoadMore, Tabs, cn } from '#lib/ui/index.js';
  import BuddyAvatar from './BuddyAvatar.svelte';
  import BuddyCompanyLogo from './BuddyCompanyLogo.svelte';
  import BuddyIdentity from './BuddyIdentity.svelte';
  import BuddyMessageButton from './BuddyMessageButton.svelte';
  import States from './States.svelte';

  // The network: members ranked by how alike their searches are, as a grid of cards, and
  // under it the companies the viewer shares with them — who else applied where you did.
  // Paged by offset; the company list is drawn from the members loaded so far.
  const PAGE = 24;
  let members = $state<BuddySimilar[]>([]);
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let busy = $state(false);
  let moreError = $state(false);
  let hasMore = $state(false);
  let filter = $state<'all' | 'companies'>('all');

  const shown = $derived(filter === 'companies' ? members.filter((m) => m.shared_companies.length > 0) : members);
  const companies = $derived(buddiesByCompany(members));

  async function load(next = false) {
    if (busy) return;
    busy = true;
    moreError = false;
    try {
      const page = await api.listBuddySimilar(PAGE, next ? members.length : 0);
      members = next ? [...members, ...page] : page;
      hasMore = page.length === PAGE;
      status = 'ready';
    } catch {
      if (next) moreError = true;
      else status = 'error';
    } finally {
      busy = false;
    }
  }
  onMount(() => { void load(); });

  // The stack of faces on a company row shows a few; the count says the rest.
  const FACES = 4;
</script>

{#if status === 'loading'}<States state="loading" />
{:else if status === 'error'}<States state="error" message="Could not load your network." onRetry={() => { status = 'loading'; void load(); }} />
{:else if members.length === 0}<States state="empty" message="No similar members yet. As more people join, the ones searching like you show up here." />
{:else}
  <div class="flex flex-col gap-4">
    <section class="flex flex-col gap-3 rounded-xl border border-border bg-card p-5">
      <div>
        <h2 class="text-base font-semibold">People like you</h2>
        <p class="text-sm text-muted-foreground">Same kind of role and place, applying to the same companies.</p>
      </div>
      <Tabs bind:value={() => filter, (next) => (filter = next as 'all' | 'companies')} tabs={[{ value: 'all', label: 'All' }, { value: 'companies', label: 'Same companies' }]}>
        {#if shown.length === 0}
          <p class="py-4 text-sm text-muted-foreground">Nobody here shares a company with you yet.</p>
        {:else}
          <ul class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {#each shown as person (person.handle)}
              <!-- A tint, not a border: a framed card inside the framed section would nest them. -->
              <li class="flex flex-col items-center gap-1 rounded-lg bg-muted p-4 text-center">
                <BuddyAvatar handle={person.handle} size="lg" class="mb-1 size-16 text-xl" />
                <BuddyIdentity handle={person.handle} summary={person.card_summary} size="sm" avatar={false} />
                <span class="mt-1 flex min-h-6 items-center justify-center gap-1.5 text-xs text-muted-foreground">
                  {#if person.shared_companies.length}
                    <span class="flex" aria-hidden="true">
                      {#each person.shared_companies.slice(0, 3) as c, i (c.slug)}
                        <BuddyCompanyLogo name={c.name} size="sm" class={cn('size-5 ring-2 ring-muted', i > 0 && '-ml-1.5')} />
                      {/each}
                    </span>
                    {buddySharedCompanies(person.shared_companies.length)}
                  {/if}
                </span>
                <!-- A column so the button stretches to the card's width. -->
                <div class="mt-2 flex w-full flex-col">
                  <BuddyMessageButton quiet handle={person.handle} conversations={messagingDock.conversations} oninvite={(h) => messagingDock.messageBuddy(h)} onconversation={(id) => messagingDock.openConversation(id)} />
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </Tabs>
      {#if hasMore}<LoadMore loading={busy} error={moreError} onclick={() => void load(true)} />{/if}
    </section>

    {#if companies.length}
      <section class="flex flex-col gap-1 rounded-xl border border-border bg-card p-5">
        <h2 class="text-base font-semibold">At the companies you applied to</h2>
        <p class="mb-2 text-sm text-muted-foreground">Members who applied where you did in the last 90 days. Ask them what the process is like.</p>
        <ul class="-mx-2 flex flex-col">
          {#each companies as group (group.company.slug)}
            <li>
              <a href={resolve('/companies/[slug]', { slug: group.company.slug })} class="group flex items-center gap-3 rounded-md px-2 py-2 hover:bg-muted">
                <BuddyCompanyLogo name={group.company.name} size="md" />
                <span class="min-w-0 flex-1 truncate font-semibold group-hover:underline group-hover:underline-offset-2">{group.company.name}</span>
                <span class="flex" aria-hidden="true">
                  {#each group.members.slice(0, FACES) as m, i (m.handle)}
                    <BuddyAvatar handle={m.handle} size="sm" class={cn('ring-2 ring-card', i > 0 && '-ml-1.5')} />
                  {/each}
                </span>
                <span class="w-20 shrink-0 text-right text-xs text-muted-foreground tabular-nums">{group.members.length === 1 ? '1 member' : `${group.members.length} members`}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </div>
{/if}
