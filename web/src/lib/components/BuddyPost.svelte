<script lang="ts">
  import { ArrowUpRight, BadgeCheck, MessagesSquare, Send } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { buddyJobLines, type BuddyPost } from '#lib/buddyFeed.js';
  import { messagingDock } from '#lib/messagingDock.svelte.js';
  import type { BuddyFeedItem } from '#lib/types.js';
  import { cn } from '#lib/ui/index.js';
  import BuddyCompanyLogo from './BuddyCompanyLogo.svelte';
  import BuddyIdentity from './BuddyIdentity.svelte';
  import BuddyJobCard from './BuddyJobCard.svelte';
  import BuddyMessageButton from './BuddyMessageButton.svelte';
  import BuddyReactionBar from './BuddyReactionBar.svelte';

  // One post of the buddy feed: who and what happened — an interview or an offer
  // with the posting as a link card, or one day's applications with the companies fanned
  // out over a short list of the postings. When is the feed's day heading's to say.
  let { post }: { post: BuddyPost } = $props();

  const handle = $derived(post.kind === 'event' ? post.item.handle : post.handle);
  const summary = $derived(post.kind === 'event' ? post.item.card_summary : post.card_summary);
  // A folded post takes its reactions on its first application, the event its key names.
  const anchor = $derived(post.kind === 'event' ? post.item : post.items[0]);

  // The further along, the more it stands out: an offer carries the brand fill.
  const STAGES = {
    applied: { icon: Send, label: 'Applied', tag: 'bg-muted text-muted-foreground' },
    interview: { icon: MessagesSquare, label: 'Interviewing', tag: 'bg-brand-muted text-brand-strong' },
    offer: { icon: BadgeCheck, label: 'Got an offer', tag: 'bg-brand text-brand-foreground' },
  } as const;

  // The tag already names the stage; the line only adds the company.
  const PREPOSITION = { applied: 'to', interview: 'at', offer: 'from' } as const;

  // The fan shows at most four marks; the list below names every posting.
  const FAN = 4;
  const TILT = ['-rotate-6', '-rotate-2', 'rotate-2', 'rotate-6'];
</script>

{#snippet jobRow(it: BuddyFeedItem)}
  {@const lines = buddyJobLines(it)}
  <BuddyCompanyLogo name={it.company.name} size="sm" class="size-6" />
  <span class="truncate font-semibold group-hover:underline group-hover:underline-offset-2">{lines.title}</span>
  <span class="truncate text-muted-foreground">{lines.where}</span>
  <ArrowUpRight class="ml-auto size-4 shrink-0 text-muted-foreground opacity-0 group-hover:opacity-100" aria-hidden="true" />
{/snippet}

<article class="flex flex-col gap-3 rounded-xl border border-border bg-card p-4">
  <header class="flex items-start justify-between gap-3">
    <BuddyIdentity {handle} {summary} />
    <!-- No date here: the feed's day heading above already says it. -->
    <div class="shrink-0">
      <BuddyMessageButton
        quiet
        {handle}
        conversations={messagingDock.conversations}
        oninvite={(h) => messagingDock.messageBuddy(h)}
        onconversation={(id) => messagingDock.openConversation(id)}
      />
    </div>
  </header>

  {#if post.kind === 'event'}
    {@const stage = STAGES[post.item.type]}
    {@const Icon = stage.icon}
    <p class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
      <span class={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold', stage.tag)}>
        <Icon class="size-3.5" aria-hidden="true" />{stage.label}
      </span>
      <span class="text-muted-foreground">{PREPOSITION[post.item.type]}</span>
      <span class="font-semibold">{post.item.company.name}</span>
    </p>
    <BuddyJobCard item={post.item} />
  {:else}
    <div class="flex items-center gap-3">
      <span class="flex shrink-0 pl-1" aria-hidden="true">
        {#each post.items.slice(0, FAN) as it, i (it.id)}
          <BuddyCompanyLogo name={it.company.name} size="lg" class={cn('ring-2 ring-card', TILT[i], i > 0 && '-ml-3')} />
        {/each}
      </span>
      <p class="min-w-0 text-sm">
        <!-- Jobs, not companies: two postings at one employer are two applications. -->
        <span class="text-muted-foreground">Applied to</span>
        <span class="font-semibold">{post.items.length} jobs</span>
        <span class="block truncate text-xs text-muted-foreground">{[...new Set(post.items.map((it) => it.company.name))].join(', ')}</span>
      </p>
    </div>
    <ul class="-mx-2 flex flex-col">
      {#each post.items as it (it.id)}
        <li>
          <!-- To the job while it has a page, to the company once it is gone (as BuddyJobCard). -->
          {#if it.job_slug}
            <a href={resolve('/jobs/[slug]', { slug: it.job_slug })} class="group flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm hover:bg-muted">{@render jobRow(it)}</a>
          {:else}
            <a href={resolve('/companies/[slug]', { slug: it.company.slug })} class="group flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm hover:bg-muted">{@render jobRow(it)}</a>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
  {#if anchor}
    <div class="-mx-1 -mb-1 border-t border-border pt-1"><BuddyReactionBar eventId={anchor.id} type={anchor.type} reactions={anchor.reactions} /></div>
  {/if}
</article>
