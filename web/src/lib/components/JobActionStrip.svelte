<script lang="ts">
  import { ExternalLink, Flag } from '@lucide/svelte';
  import { Button } from '#lib/ui/index.js';
  import JobSaveButton from './JobSaveButton.svelte';

  let {
    className,
    iconOnly = false,
    saved,
    onSaveClick,
    onReportClick,
    applyUrl,
    applyLabel,
    onApplyClick,
  }: {
    className: string;
    iconOnly?: boolean;
    saved: boolean;
    onSaveClick: () => void;
    onReportClick: () => void;
    /** The posting's own page, linked here only while auto-apply leads the bottom bar. */
    applyUrl: string | null;
    applyLabel: string;
    onApplyClick: (event: MouseEvent) => void;
  } = $props();
</script>

<!-- The action strip: the QUIET things a reader does with a posting — talk about it, flag
     it, keep it, file it — on one line, sharing the tab row's rule. It carries the
     rule itself (`border-b`) rather than sitting above one, so the line reads as a
     single edge across the column: the strip and the TabStrip beside it are aligned on
     their bottoms, and each draws its own half of it.
     The two loud things — auto-apply and the apply link — are NOT here; they are ctaGroup,
     up beside the title. They shared this row until there were six controls on it, at which
     point the TabStrip, the only half that yields, had been squeezed to a scrolling sliver.
     Rendered in two places, one visible at a time (the caller passes the display class):
     the tab row on lg, and directly under the title below it, where the sidebar stacks
     between the title and the description and a strip left on the tab row would put Save
     a whole screen away from the job it saves. -->
<div class={`shrink-0 items-center gap-1.5 ${className}`}>
  <!-- Report, the same shape as Save and, like it, `ghost`: complaining about a posting is
       the rarest thing on the strip, and a filled or outlined box would give it the
       standing of the apply CTA beside it. -->
  <Button
    variant="ghost"
    size="sm"
    onclick={onReportClick}
    aria-label="Report this job"
    title="Report this job"
    class="shrink-0 gap-1.5 px-2 text-muted-foreground"
  >
    <Flag class="size-4 shrink-0" aria-hidden="true" />
    <!-- Icon only from lg, where it sits beside the CTAs: a rare action, it should not
         weigh as much as Apply. The accessible name stays on the button. -->
    <span class="sr-only">Report</span>
  </Button>
  <JobSaveButton {saved} onclick={onSaveClick} className={iconOnly ? 'size-11 px-0' : ''} {iconOnly} />
  <!-- The link out, phone only. Below lg the sticky bottom bar belongs to auto-apply
       whenever auto-apply can be started, so the posting's own page needs a door
       somewhere else — and this strip is where the quiet things already are. On lg the
       same link is a button up beside the title, so this copy stays hidden rather than
       offering it twice. Same handler as that button: one click, one apply-intent event,
       whichever copy the reader reached.
       An anchor rather than JobView's `applyCta`, because a `Button` here would be a fourth loud
       thing on a strip whose whole job is to be quiet — so it carries `rel` and `target`
       itself. `nofollow` for the reason `applyCta` states in full: the destination
       is the posting's own site, which the catalogue never vetted, and without it a
       submitted vacancy buys a followed link from every job page. Change one, change
       both. -->
  {#if applyUrl}
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the posting's own URL on its employer's site; there is no route to resolve --><a
      href={applyUrl}
      class="inline-flex shrink-0 items-center gap-1.5 px-2 text-sm font-medium text-muted-foreground hover:text-foreground lg:hidden"
      target="_blank"
      rel="nofollow noopener noreferrer"
      onclick={onApplyClick}
    >
      <ExternalLink class="size-4 shrink-0" aria-hidden="true" />
      {applyLabel}
    </a>
  {/if}
</div>
