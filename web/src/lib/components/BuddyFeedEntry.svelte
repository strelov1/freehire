<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { buddyFeedEventLabel } from '#lib/buddies.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import type { BuddyFeedItem } from '#lib/types.js';
  import { EntityLogo } from '#lib/ui/index.js';

  // One line of a buddy feed: who (unless the page is already theirs), what happened, at
  // which company and posting, and the day — never a time.
  let { item, showHandle = true, actions }: { item: BuddyFeedItem; showHandle?: boolean; actions?: Snippet } = $props();
</script>

<li class="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-border py-3 text-sm">
  {#if showHandle}
    <a href={resolve('/my/buddies/[handle]', { handle: item.handle })} class="font-medium hover:underline">{item.handle}</a>
  {/if}
  <span>{buddyFeedEventLabel(item.type)}</span>
  <span class="flex min-w-0 items-center gap-1.5">
    <EntityLogo name={item.company.name} src={companyLogoUrl(item.company.name) ?? undefined} shape="square" size="xs" />
    <span class="font-medium">{item.company.name}</span>
  </span>
  {#if item.job_title}<span class="min-w-0 text-muted-foreground">· {item.job_title}</span>{/if}
  <span class="ml-auto flex items-center gap-3">
    <time datetime={item.day} class="text-xs text-muted-foreground">{item.day}</time>
    {@render actions?.()}
  </span>
</li>
