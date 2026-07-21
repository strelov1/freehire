<script lang="ts">
  import { countryLabel } from '$lib/facets';
  import { cn } from '$lib/utils';

  // A round country flag, backed by the flag-icons sheet (imported once in the root
  // layout) and shared by the job summary, the location/company filters and company
  // cards. `code` is an ISO 3166-1 alpha-2 code in any case; the flag scales with the
  // surrounding font size (flag-icons sizes in `em`). `.fis` picks the square (1:1)
  // variant so `rounded-full` yields a circle; the ring keeps white flags (Japan,
  // Nigeria) visible on a light background.
  //
  // Codes that aren't two ASCII letters have no flag in the sheet and would render an
  // empty box — fall back to the upper-cased code as plain text instead, so the value
  // is never worse than the bare code we showed before.
  let { code, class: className = '' }: { code: string; class?: string } = $props();

  const cc = $derived(code.trim().toLowerCase());
  const renderable = $derived(/^[a-z]{2}$/.test(cc));
  const name = $derived(countryLabel(code));
</script>

{#if renderable}
  <span
    class={cn('fi fis shrink-0 rounded-full ring-1 ring-black/10 dark:ring-white/15', `fi-${cc}`, className)}
    role="img"
    aria-label={name}
    title={name}
  ></span>
{:else}
  <span class={className} title={name}>{code.toUpperCase()}</span>
{/if}
