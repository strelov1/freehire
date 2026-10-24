<script lang="ts">
  import type { ClientMatch } from '#lib/jobMatch.js';

  // A card-level profile-match strip: a thin coverage bar + "N% · matched/total skills".
  // Purely presentational — the owning JobRow computes the client-side match (exact
  // overlap of the job's skills and the signed-in viewer's profile skills) and passes it
  // in, so the chips it colours and this bar can't disagree on the score. `match` is null
  // when there is nothing to show, in which case nothing renders.
  //
  // `blurred` renders the same strip as a teaser for a viewer who has no match yet — a
  // guest, or a signed-in viewer with no profile skills. The figures are then the job's
  // deterministic teaser rather than a computed score, so they are lightly blurred and
  // hidden from assistive technology: a fabricated percentage must not be read out as
  // this viewer's match. Offering the text alternative is the caller's job — only it
  // knows which invitation applies, and where it can sit without joining the accessible
  // name of a card-wide link.
  let {
    match,
    blurred = false,
  }: { match: ClientMatch | null; blurred?: boolean } = $props();
</script>

{#if match}
  <div
    class={[
      'mt-3 flex items-center gap-2 border-t border-dashed border-border pt-2.5',
      blurred && 'pointer-events-none select-none opacity-90 blur-[1.5px]',
    ]}
    aria-hidden={blurred ? 'true' : undefined}
  >
    <!-- The spoken form of the strip. An `aria-label` on this plain div was ignored — a
         generic element has no name to give — so assistive technology heard the terse
         "62% · 5/8 skills" below. The visible figure is hidden from it instead, and this
         says the same thing in words. Not rendered under the blur, where the whole strip
         is hidden (see above). -->
    {#if !blurred}
      <span class="sr-only">Profile match: {match.percent}%, {match.matched} of {match.total} skills</span>
    {/if}
    <!-- The unfilled remainder is a soft red (the skills you're missing); the fill is the
         brand tone (the skills you have) — a two-tone have/missing bar in one track.
         The fill is full-width and slid left by the missing share, not sized by `width`:
         a transform animates on the compositor, where a width change re-lays out every
         row in the feed on each frame. Sliding rather than `scale-x` keeps the rounded
         leading edge round; the track's own clip draws the trailing one. -->
    <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-destructive/15">
      <div
        class="h-full rounded-full bg-brand transition-transform"
        style:transform="translateX({match.percent - 100}%)"
      ></div>
    </div>
    <span class="shrink-0 text-xs font-medium tabular-nums text-muted-foreground" aria-hidden="true">
      {match.percent}% · {match.matched}/{match.total} skills
    </span>
  </div>
{/if}
