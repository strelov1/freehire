<script lang="ts">
  import { Check, Eye } from '@lucide/svelte';
  import { formatCount } from '#lib/utils.js';

  // The facts at the end of a job card's rail: whether the reader has viewed it, the public
  // view count, the posting date (`posted`, when `showPosted`) and the application deadline.
  // `touchActions` hides the date on touch, where the meta line carries it instead.
  let {
    isViewed,
    views,
    posted,
    showPosted,
    deadline,
    touchActions,
  }: {
    isViewed: boolean;
    views: number;
    posted: string;
    showPosted: boolean;
    deadline: string | null | undefined;
    touchActions: boolean;
  } = $props();
</script>

<div class="flex shrink-0 items-center gap-1.5 text-muted-foreground">
  {#if isViewed}
    <!-- A quiet "you've seen this" marker, paired with the lighter dim on the card
       so viewed jobs recede without becoming hard to read. The dim is 90%, not
       80%: on the hover fill (`accent`) 80% took muted text to 4.15:1 in the light
       theme, under the 4.5:1 floor; 90% keeps it at 5.2:1 in both themes.
       A check, not an eye: the eye beside it carries the PUBLIC view count, and
       two eyes in one rail meaning different things cannot be told apart. -->
    <Check class="size-3.5" aria-label="You have viewed this" />
  {/if}
  {#if views > 0}
    <!-- Abbreviated only because the rail shares its width with a company name that
       truncates, and a five-digit figure spelled out crowds the name off the card.
       A screen reader has no such constraint, so it hears the exact figure — the
       same one the job's own page shows. -->
    <span class="flex items-center gap-1 text-xs tabular-nums">
      <Eye class="size-3.5" aria-hidden="true" />
      <span aria-hidden="true">{formatCount(views)}</span>
      <span class="sr-only">{views} views</span>
    </span>
  {/if}
  {#if showPosted}
    <span class={['text-xs tabular-nums', touchActions && 'pointer-coarse:hidden']}>{posted}</span>
  {/if}
  {#if deadline}
    <span class="whitespace-nowrap text-xs tabular-nums">
      Apply by <time datetime={deadline}>{deadline}</time>
    </span>
  {/if}
</div>
