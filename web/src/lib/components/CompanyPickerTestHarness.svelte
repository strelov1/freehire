<script lang="ts">
  import CompanyPicker from './CompanyPicker.svelte';

  // Test-only: exercises `CompanyPicker`'s `bind:value` through a real Svelte binding
  // instead of introspecting the mounted instance, and renders the bound text into the
  // DOM so CompanyPicker.spec.ts can assert on it with ordinary testing-library queries.
  let { onSelect }: { onSelect?: (company: { slug: string; name: string } | null) => void } = $props();

  let value = $state('');
</script>

<CompanyPicker bind:value {onSelect} />
<span data-testid="picker-value">{value}</span>
