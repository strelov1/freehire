<script lang="ts">
  import type { OnboardingNudge } from '#lib/onboardingNudge.svelte.js';
  import MatchNeedsSkillsNotice from './MatchNeedsSkillsNotice.svelte';
  import MatchSortHint from './MatchSortHint.svelte';
  import OnboardingAlertBanner from './onboarding/OnboardingAlertBanner.svelte';
  import OnboardingBanner from './onboarding/OnboardingBanner.svelte';

  // What the feed says above its rows without replacing them: the onboarding nudges and
  // the match-sort hints.
  let {
    nudge,
    matchSortHint,
    minMatch,
    onSortByMatch,
    matchNeedsSkills,
  }: {
    nudge: OnboardingNudge;
    matchSortHint: boolean;
    minMatch: number | null;
    onSortByMatch: () => void;
    matchNeedsSkills: boolean;
  } = $props();
</script>

<!-- Onboarding nudges sit UNDER the toolbar so the feed controls stay at the top;
     each shows once (until dismissed or completed), then retires. Never blocks
     the feed below. -->
{#if nudge.show || nudge.alert}
  <div class="mt-3">
    {#if nudge.show}
      <OnboardingBanner onOpen={nudge.open} onDismiss={nudge.dismiss} />
    {/if}
    {#if nudge.alert}
      <OnboardingAlertBanner
        query={nudge.alert.query}
        autostart={nudge.alert.autostart}
        onDismiss={() => (nudge.alert = null)}
      />
    {/if}
  </div>
{/if}

<!-- Match was chosen and there is nothing to rank against. Sits with the nudges
     above and, like them, never replaces the feed: the server degrades this request
     to newest rather than refusing it, so there ARE jobs below — taking them away
     would punish someone for pressing a button we offered them. What was missing
     was only the explanation. -->
{#if matchSortHint}
  <MatchSortHint {minMatch} onSort={onSortByMatch} />
{/if}
{#if matchNeedsSkills}
  <MatchNeedsSkillsNotice />
{/if}
