<script lang="ts">
  import type { ClientMatch } from '$lib/jobMatch';

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
    aria-label={blurred
      ? undefined
      : `Profile match: ${match.percent}%, ${match.matched} of ${match.total} skills`}
    aria-hidden={blurred ? 'true' : undefined}
  >
    <!-- The unfilled remainder is a soft red (the skills you're missing); the fill is the
         brand tone (the skills you have) — a two-tone have/missing bar in one track. -->
    <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-destructive/15">
      <div class="h-full rounded-full bg-brand transition-all" style="width: {match.percent}%"></div>
    </div>
    <span class="shrink-0 text-xs font-medium tabular-nums text-muted-foreground">
      {match.percent}% · {match.matched}/{match.total} skills
    </span>
  </div>
{/if}
