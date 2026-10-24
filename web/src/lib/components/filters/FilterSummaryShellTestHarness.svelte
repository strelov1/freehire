<script lang="ts">
  import { untrack } from 'svelte';
  import FilterSummaryShell, { type SummaryGroup } from './FilterSummaryShell.svelte';

  // A live store stand-in for FilterSummaryShell.spec.ts: removing a chip really
  // removes it, so the spec can watch where focus lands once the button is gone.
  let { values: initial }: { values: string[] } = $props();
  let values = $state(untrack(() => [...initial]));

  const groups = $derived<SummaryGroup[]>(
    values.length === 0
      ? []
      : [
          {
            label: 'Skills',
            chips: values.map((v) => ({
              key: v,
              text: v,
              exclude: false,
              remove: () => (values = values.filter((x) => x !== v)),
            })),
          },
        ],
  );
</script>

<FilterSummaryShell {groups} active={values.length} onReset={() => (values = [])} onOpen={() => {}} />
