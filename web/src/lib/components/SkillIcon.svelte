<script lang="ts">
  import { BrandMark } from '#lib/ui/index.js';
  import { ensureTechMarks, techMarks } from '#lib/techmarksLoad.svelte.js';

  // The one place a skill chip turns its slug into a brand logo — every call site
  // that renders a skill value (job cards, the skills facet picker, the profile's
  // skill picker) drops this in unconditionally. Coverage is intentionally partial
  // (see techmarks.ts): a slug with no entry renders nothing, same as before this
  // component existed, rather than a generic placeholder that would outnumber the
  // real logos.
  let { slug, class: className = 'size-3 shrink-0' }: { slug: string; class?: string } = $props();

  const onBrowser = typeof window !== 'undefined';
  if (onBrowser) ensureTechMarks();

  const mark = $derived(onBrowser ? techMarks()?.[slug] : undefined);
</script>

{#if mark}
  <BrandMark path={mark.path} hex={mark.hex} title={mark.title} class={className} />
{/if}
