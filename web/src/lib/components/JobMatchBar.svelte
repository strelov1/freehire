<script lang="ts">
  import type { ClientMatch } from '$lib/jobMatch';

  // A card-level profile-match strip: a thin coverage bar + "N% · matched/total skills".
  // Purely presentational — the owning JobRow computes the client-side match (exact
  // overlap of the job's skills and the signed-in viewer's profile skills) and passes it
  // in, so the chips it colours and this bar can't disagree on the score. `match` is null
  // for guests / no-profile viewers, in which case nothing renders. `gutterRight` reserves
  // the feed's bottom-right hide control so the percent never slides under that icon.
  let { match, gutterRight = false }: { match: ClientMatch | null; gutterRight?: boolean } = $props();
</script>

{#if match}
  <div
    class={['mt-3 flex items-center gap-2 border-t border-dashed border-border pt-2.5', gutterRight && 'pr-9']}
    aria-label="Profile match: {match.percent}%, {match.matched} of {match.total} skills"
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
