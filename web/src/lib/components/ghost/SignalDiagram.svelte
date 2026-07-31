<script lang="ts">
  import type { Component } from 'svelte';
  import type { GhostCriterionCode } from '$lib/ghost';
  import AtsAbsent from './diagrams/AtsAbsent.svelte';
  import Evergreen from './diagrams/Evergreen.svelte';
  import Reports from './diagrams/Reports.svelte';
  import Silent from './diagrams/Silent.svelte';

  // The registry is keyed by the criterion union rather than by `string`, so a criterion
  // that joins the classifier's vocabulary without an illustration is a COMPILE error
  // (TS2741) instead of an empty cell on the page.
  //
  // This is what stands in for the test that cannot be written: vitest here cannot import
  // a `.svelte` component — the `$lib` alias does not resolve under it, and a relative
  // path reaches vite's import analysis untransformed — so asserting the registry's
  // coverage at runtime would mean adding test infrastructure this project does not
  // carry. The type checker already runs in CI and catches it earlier anyway.
  const DIAGRAMS: Record<GhostCriterionCode, Component> = {
    evergreen_posting: Evergreen,
    ats_absent: AtsAbsent,
    silent_applications: Silent,
    user_reports: Reports,
  };

  let { code }: { code: GhostCriterionCode } = $props();

  const Diagram = $derived(DIAGRAMS[code]);
</script>

<Diagram />
