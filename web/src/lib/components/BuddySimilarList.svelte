<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { buddySharedCompanies } from '#lib/buddyPresent.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import type { BuddySimilar } from '#lib/types.js';
  import { Card, Chip, EntityLogo } from '#lib/ui/index.js';
  import BuddyIdentity from './BuddyIdentity.svelte';
  import BuddyMessageButton from './BuddyMessageButton.svelte';
  import States from './States.svelte';

  // Members ranked by how alike their searches are, with the companies both applied to.
  const similar = new AsyncData<BuddySimilar[]>([]);
  async function load() { await similar.run(() => api.listBuddySimilar()); }
  onMount(() => { void load(); });
</script>

{#if similar.status === 'loading'}<States state="loading" />
{:else if similar.status === 'error'}<States state="error" message="Could not load similar members." onRetry={() => void load()} />
{:else if similar.value.length === 0}<States state="empty" message="No similar members yet." />{:else}
  <ul class="flex flex-col gap-3">
    {#each similar.value as person (person.handle)}
      <li>
        <Card class="flex flex-col gap-3 p-4">
          <div class="flex items-start justify-between gap-3">
            <BuddyIdentity handle={person.handle} summary={person.card_summary} />
            <BuddyMessageButton quiet handle={person.handle} conversations={messagingDock.conversations} oninvite={(h) => messagingDock.messageBuddy(h)} onconversation={(id) => messagingDock.openConversation(id)} />
          </div>
          {#if person.shared_companies.length}
            <div class="flex flex-col gap-1.5 sm:pl-13">
              <p class="text-xs text-muted-foreground">{buddySharedCompanies(person.shared_companies.length)}</p>
              <div class="flex flex-wrap gap-1.5">
                {#each person.shared_companies as company (company.slug)}
                  <Chip class="gap-1.5 py-1 pl-1"><EntityLogo name={company.name} src={companyLogoUrl(company.name) ?? undefined} shape="square" size="xs" />{company.name}</Chip>
                {/each}
              </div>
            </div>
          {/if}
        </Card>
      </li>
    {/each}
  </ul>
{/if}
