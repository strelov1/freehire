<script lang="ts">
  import type { Snippet } from 'svelte';
  import { BadgeCheck, MessagesSquare, Send } from '@lucide/svelte';
  import { buddyFeedEventLabel } from '#lib/buddies.js';
  import { buddyRelativeDay, localDay } from '#lib/buddyPresent.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import type { BuddyFeedItem } from '#lib/types.js';
  import { Card, EntityLogo } from '#lib/ui/index.js';
  import BuddyIdentity from './BuddyIdentity.svelte';

  // One event of a buddy feed. On the feed it is a card led by who it was; on a member's
  // own page (`showHandle` false) the person is already the page, so it is a compact row.
  // Either way: what happened, at which company and posting, and the DAY — the API carries
  // no time, so nothing here may suggest one.
  let { item, showHandle = true, actions }: { item: BuddyFeedItem; showHandle?: boolean; actions?: Snippet } = $props();

  // The type cue: an offer carries the brand accent, an interview the secondary tint, an
  // application stays neutral — the further along, the more it stands out.
  const CUES = {
    applied: { icon: Send, tile: 'bg-muted text-muted-foreground' },
    interview: { icon: MessagesSquare, tile: 'bg-secondary text-secondary-foreground' },
    offer: { icon: BadgeCheck, tile: 'bg-brand-muted text-brand-strong' },
  } as const;

  const cue = $derived(CUES[item.type]);
  const when = $derived(buddyRelativeDay(item.day, localDay()));
</script>

{#snippet event()}
  {@const Icon = cue.icon}
  <div class="flex min-w-0 flex-1 items-start gap-3">
    <span class="flex size-8 shrink-0 items-center justify-center rounded-md {cue.tile}">
      <Icon class="size-4" aria-hidden="true" />
    </span>
    <div class="flex min-w-0 flex-col gap-0.5">
      <p class="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-sm">
        <span class={item.type === 'offer' ? 'font-medium text-brand-strong' : 'text-muted-foreground'}>{buddyFeedEventLabel(item.type)}</span>
        <span class="flex min-w-0 items-center gap-1.5">
          <EntityLogo name={item.company.name} src={companyLogoUrl(item.company.name) ?? undefined} shape="square" size="xs" />
          <span class="font-medium text-foreground">{item.company.name}</span>
        </span>
      </p>
      {#if item.job_title}<p class="truncate text-sm text-muted-foreground">{item.job_title}</p>{/if}
    </div>
  </div>
{/snippet}

{#snippet day()}
  <time datetime={item.day} title={item.day} class="shrink-0 text-xs text-muted-foreground">{when}</time>
{/snippet}

{#if showHandle}
  <li>
    <Card class="flex flex-col gap-3 p-4">
      <div class="flex items-start justify-between gap-3">
        <BuddyIdentity handle={item.handle} summary={item.card_summary} />
        <div class="flex shrink-0 items-center gap-3">
          {@render day()}
          {@render actions?.()}
        </div>
      </div>
      <div class="sm:pl-13">{@render event()}</div>
    </Card>
  </li>
{:else}
  <li class="flex items-start gap-3 border-b border-border py-3 last:border-b-0">
    {@render event()}
    {@render day()}
    {@render actions?.()}
  </li>
{/if}
