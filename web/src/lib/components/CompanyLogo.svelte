<script lang="ts">
  import { Globe } from '@lucide/svelte';
  import { logoDevUrl } from '$lib/logo';

  // A company's logo from logo.dev's name endpoint. `fallback=404` makes the API
  // return 404 when it has no logo, so the image fails to load and we fall back to
  // our own monogram (the initial on a colour derived from the name). The globe is
  // the last resort, only when there's no name to monogram. Shared by the job row,
  // the job page, and the company page for one look (and by the OG-image card, via
  // $lib/logo).
  let { name, size = 'size-4' }: { name: string; size?: string } = $props();

  let failed = $state(false);
  const src = $derived(logoDevUrl(name) ?? undefined);

  // A new company means a fresh attempt — clear a prior failure when the name changes
  // (the company page reuses this instance across navigations).
  $effect(() => {
    void name; // re-run when the company changes
    failed = false;
  });

  // SSR sends the raw <img>, so the browser fetches the logo before hydration wires
  // `onerror`. When logo.dev 404s (its `fallback=404`), that error event fires before
  // the handler exists and is lost — leaving an empty broken image until a reload. On
  // mount, an already-complete image with no intrinsic width is that missed failure,
  // so fall back to the monogram straight away.
  function catchMissedError(node: HTMLImageElement) {
    if (node.complete && node.naturalWidth === 0) failed = true;
  }

  // Monogram fallback: the first letter over a colour hashed from the full name, so
  // a company keeps the same tile everywhere. Drawn as SVG so the glyph scales
  // crisply with whatever `size` the caller passes (16–64px).
  const initial = $derived((name.trim()[0] ?? '').toUpperCase());
  const hue = $derived.by(() => {
    let h = 0;
    for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) % 360;
    return h;
  });
</script>

{#if name && !failed}
  <img
    {src}
    alt=""
    class="{size} shrink-0 rounded object-contain"
    loading="lazy"
    onerror={() => (failed = true)}
    {@attach catchMissedError}
  />
{:else if initial}
  <svg class="{size} shrink-0" viewBox="0 0 32 32" aria-hidden="true">
    <rect width="32" height="32" rx="7" fill="hsl({hue} 52% 45%)" />
    <text
      x="16"
      y="16"
      text-anchor="middle"
      dominant-baseline="central"
      fill="#fff"
      font-size="16"
      font-weight="600"
      font-family="inherit">{initial}</text>
  </svg>
{:else}
  <Globe class="{size} shrink-0 text-muted-foreground" />
{/if}
