<script lang="ts">
  // The PARKED PRODUCT INTRODUCTION from the old onboarding tour (replaced by the CV-first
  // flow on /onboarding, 2026-11-12): what helps with what the account said is in its way,
  // and the three intro screens — the AI connector, CV tailoring and the browser extension
  // — as sections of one page. Not linked from navigation yet: it is kept to be refined
  // and placed in the product, not shown as is.
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import AccountPageHeading from '#lib/components/AccountPageHeading.svelte';
  import AgentStep from '#lib/components/onboarding/AgentStep.svelte';
  import ChallengeTips from '#lib/components/onboarding/ChallengeTips.svelte';
  import ExtensionStep from '#lib/components/onboarding/ExtensionStep.svelte';
  import TailorStep from '#lib/components/onboarding/TailorStep.svelte';

  // The stored "in your way" answers; a failed read just shows no tips.
  let challenges = $state.raw<string[]>([]);
  onMount(() => {
    api
      .getSurvey()
      .then((s) => (challenges = s.biggest_challenges ?? []))
      .catch(() => {});
  });
</script>

<svelte:head>
  <title>Getting started · freehire</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="flex flex-col gap-10">
  <div class="flex flex-col gap-1">
    <AccountPageHeading>Getting started</AccountPageHeading>
    <p class="text-sm text-muted-foreground">What freehire can do for your search beyond the feed.</p>
  </div>

  {#if challenges.length > 0}
    <ChallengeTips picked={challenges} />
  {/if}

  <section><AgentStep /></section>
  <section><TailorStep /></section>
  <section><ExtensionStep /></section>
</div>
