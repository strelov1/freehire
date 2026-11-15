<script lang="ts">
  import { onMount } from 'svelte';
  import { sourceFamily, sourceLabel } from '#lib/sourceLabels.js';
  import { SOURCE_LOGO_DOMAINS, sourceLogoUrl } from '#lib/logo.js';
  import { loadSourceKinds } from '#lib/sourceKinds.js';
  import type { ProviderKind } from '#lib/types.js';
  import { EntityLogo } from '#lib/ui/index.js';

  let { source, framed = true }: { source: string; framed?: boolean } = $props();
  const family = $derived(sourceFamily(source.toLowerCase()));
  const label = $derived(sourceLabel(family));
  let kinds = $state.raw<ReadonlyMap<string, ProviderKind>>(new Map());
  const kind = $derived(kinds.get(family));
  const labels: Record<ProviderKind, string> = {
    ats: 'ATS',
    aggregator: 'Aggregator',
    company: 'Company',
    social: 'Community',
    other: 'Other',
  };

  onMount(() => {
    let active = true;
    void loadSourceKinds().then((result) => {
      if (active) kinds = result;
    });
    return () => {
      active = false;
    };
  });
</script>

<span
  class={[
    'inline-flex max-w-full items-center gap-1.5 whitespace-nowrap text-xs font-medium text-muted-foreground',
    framed && 'rounded-md border border-border/70 bg-background px-1.5 py-0.5',
  ]}
>
  <EntityLogo
    name={label}
    src={sourceLogoUrl(label, SOURCE_LOGO_DOMAINS[family]) ?? undefined}
    shape="square"
    size="xs"
    class="shrink-0"
  />
  <span class="truncate">{label}</span>
  {#if kind}
    <span class="shrink-0 border-l border-border pl-1.5 text-xs font-normal">{labels[kind]}</span>
  {/if}
</span>
