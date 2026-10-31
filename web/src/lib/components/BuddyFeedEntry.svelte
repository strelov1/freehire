<script lang="ts">
  import { BadgeCheck, MessagesSquare, Send } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { buddyFeedEventLabel } from '#lib/buddies.js';
  import { buddyRelativeDay, localDay } from '#lib/buddyPresent.js';
  import type { BuddyFeedItem } from '#lib/types.js';
  import BuddyCompanyLogo from './BuddyCompanyLogo.svelte';

  // One event on a member's own page, as a compact row: the person is already the page, so
  // the row says what happened, at which company and posting (a link while the posting has
  // a page), and the DAY — the API carries no time, so nothing here may suggest one. The
  // feed draws the same events as posts (BuddyPost).
  let { item }: { item: BuddyFeedItem } = $props();

  // The type cue: an offer carries the brand accent, an interview the brand tint, an
  // application stays neutral — the further along, the more it stands out.
  const CUES = {
    applied: { icon: Send, tile: 'bg-muted text-muted-foreground' },
    interview: { icon: MessagesSquare, tile: 'bg-brand-muted text-brand-strong' },
    offer: { icon: BadgeCheck, tile: 'bg-brand text-brand-foreground' },
  } as const;

  const cue = $derived(CUES[item.type]);
  const Icon = $derived(cue.icon);
  const when = $derived(buddyRelativeDay(item.day, localDay()));
</script>

<li class="flex items-start gap-3 border-b border-border py-3 last:border-b-0">
  <span class="flex size-8 shrink-0 items-center justify-center rounded-full {cue.tile}">
    <Icon class="size-4" aria-hidden="true" />
  </span>
  <div class="flex min-w-0 flex-1 flex-col gap-0.5">
    <p class="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-sm">
      <span class={item.type === 'offer' ? 'font-medium text-brand-strong' : 'text-muted-foreground'}>{buddyFeedEventLabel(item.type)}</span>
      <span class="flex min-w-0 items-center gap-1.5">
        <BuddyCompanyLogo name={item.company.name} size="sm" class="size-5" />
        <span class="font-medium text-foreground">{item.company.name}</span>
      </span>
    </p>
    {#if item.job_title && item.job_slug}
      <a href={resolve('/jobs/[slug]', { slug: item.job_slug })} class="truncate text-sm text-muted-foreground hover:text-foreground hover:underline">{item.job_title}</a>
    {:else if item.job_title}
      <p class="truncate text-sm text-muted-foreground">{item.job_title}</p>
    {/if}
  </div>
  <time datetime={item.day} title={item.day} class="shrink-0 text-xs text-muted-foreground">{when}</time>
</li>
