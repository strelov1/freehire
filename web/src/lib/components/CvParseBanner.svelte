<script lang="ts">
  import { Button } from '#lib/ui/index.js';
  import type { CvParseState } from '#lib/onboardingResumeWait.js';

  // Whether the candidate's CV could be read, said where they manage their experience.
  let { state, onRetry }: { state: CvParseState; onRetry: () => void } = $props();
</script>

{#if state === 'failed'}
  <!-- The CV's parse_status, surfaced where a candidate actually manages experience
         rather than only once at sign-up. -->
  <div class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg bg-warning/5 px-4 py-3 text-sm">
    <p>Couldn't read your CV — that may be why this looks empty.</p>
    <Button size="sm" variant="outline" onclick={onRetry}>Try again</Button>
  </div>
{:else if state === 'waiting'}
  <div class="mb-4 rounded-lg bg-muted/40 px-4 py-3 text-sm text-muted-foreground">Reading your CV again…</div>
{/if}
