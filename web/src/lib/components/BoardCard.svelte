<script lang="ts">
  import { Clock, Mail, MessageSquare, Mic, Send } from '@lucide/svelte';
  import CompanyLogo from './CompanyLogo.svelte';
  import { Badge } from '$lib/ui';
  import { humanizeStage } from '$lib/stages';
  import { canFollowUp, chasedLabel } from '$lib/followup';
  import { canRehearse } from '$lib/rehearsal';
  import type { MyJob } from '$lib/types';

  let {
    item,
    onopen,
    onfollowup,
    onrehearse,
  }: {
    item: MyJob;
    onopen: (item: MyJob) => void;
    onfollowup: (item: MyJob) => void;
    onrehearse: (item: MyJob) => void;
  } = $props();

  const hasNotes = $derived(!!item.notes && item.notes.trim().length > 0);
  // Both readings of a chased application, side by side: the badge says the employer
  // is still quiet, the label says we already prodded them. Neither replaces the other.
  const offersFollowUp = $derived(canFollowUp(item));
  const chased = $derived(chasedLabel(item));
  // Both can show at once: `screening` tolerates 18 days of silence, so an application
  // waiting on a scheduled call that has gone quiet offers a chase AND a rehearsal. The
  // row wraps, and the rehearsal is listed first because it is the one with a date on it.
  const offersRehearsal = $derived(canRehearse(item));
</script>

<!-- The card is a div, not a button: the follow-up action is a second control, and a
     button inside a button is invalid. The open action keeps the whole card clickable
     via a stretched ::after overlay; the follow-up button sits above it on z-10. -->
<div
  class="relative flex w-full flex-col gap-1.5 rounded-lg border border-border bg-card p-3 text-left shadow-sm transition-colors hover:bg-accent"
>
  <button
    type="button"
    onclick={() => onopen(item)}
    aria-label="Open {item.job.title} at {item.job.company || 'unknown company'}"
    class="flex flex-col gap-1.5 text-left after:absolute after:inset-0 after:content-['']"
  >
    <span class="flex items-center gap-1.5 text-sm font-semibold">
      <CompanyLogo name={item.job.company} />
      <span class="min-w-0 truncate">{item.job.company || 'Unknown company'}</span>
    </span>
    <span class="line-clamp-2 text-sm">{item.job.title}</span>
  </button>
  <!-- The badge row rides above the open action's overlay so its title tooltips still
       answer a hover. The cost is that this strip no longer opens the drawer; a lost
       tooltip is the worse trade, since the badges are the card's only explanation of
       what "24d" means. -->
  <span class="relative z-10 flex flex-wrap items-center gap-x-1.5 gap-y-1">
    {#if item.stage}
      <Badge variant="secondary">{humanizeStage(item.stage)}</Badge>
    {/if}
    {#if item.silence_state === 'silent'}
      <span
        class="flex items-center gap-0.5 text-xs tabular-nums text-warning-strong"
        title="No reply for {item.days_silent} days"
        aria-label="No reply for {item.days_silent} days"
      >
        <Clock class="size-3 shrink-0" aria-hidden="true" />
        {item.days_silent}d
      </span>
    {:else if item.silence_state === 'unconfirmed'}
      <span
        class="flex items-center gap-0.5 text-xs text-muted-foreground"
        title="Mail may be from them — confirm the link to know"
        aria-label="Mail awaiting confirmation"
      >
        <MessageSquare class="size-3 shrink-0" aria-hidden="true" />
        ?
      </span>
    {/if}
    {#if chased}
      <span class="text-xs text-muted-foreground">{chased}</span>
    {/if}
    {#if item.email_count > 0}
      <span
        class="flex items-center gap-0.5 text-xs tabular-nums text-muted-foreground"
        title="{item.email_count} linked email{item.email_count === 1 ? '' : 's'}"
        aria-label="{item.email_count} linked email{item.email_count === 1 ? '' : 's'}"
      >
        <Mail class="size-3 shrink-0" aria-hidden="true" />
        {item.email_count}
      </span>
    {/if}
    {#if hasNotes}
      <svg
        class="size-3 shrink-0 text-muted-foreground"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        role="img"
        aria-label="Has notes"
      >
        <title>Has notes</title>
        <path d="M8 7h8M8 12h8M8 17h5" />
      </svg>
    {/if}
    {#if offersRehearsal}
      <button
        type="button"
        onclick={() => onrehearse(item)}
        class="relative z-10 ml-auto flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <Mic class="size-3 shrink-0" aria-hidden="true" />
        Rehearse
      </button>
    {/if}
    {#if offersFollowUp}
      <button
        type="button"
        onclick={() => onfollowup(item)}
        class:ml-auto={!offersRehearsal}
        class="relative z-10 flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs font-medium text-warning-strong transition-colors hover:bg-warning-muted text-warning-strong dark:hover:bg-warning-muted/50"
      >
        <Send class="size-3 shrink-0" aria-hidden="true" />
        {chased ? 'Chase again' : 'Follow up'}
      </button>
    {/if}
  </span>
</div>
