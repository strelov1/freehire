<script lang="ts">
  // A brand's own logo glyph, drawn from its official SVG path + hex color — the
  // `simple-icons` shape, but this primitive takes the two fields directly rather
  // than depending on that package: the caller (freehire's `techmarks.ts`) is the
  // one that knows which slug maps to which mark, and stays free to source marks
  // from wherever it needs to (simple-icons today, a hand-drawn path tomorrow).
  // Unlike ProviderIcon (a fixed switch over a handful of OAuth brands, several of
  // which follow the surrounding text color instead of a fixed brand color), every
  // mark here always renders in its own brand color — flat on the page, not on a
  // colored badge — since the call sites are compact chips, not standalone marks.
  let { path, hex, title, class: className = 'size-4' }: { path: string; hex: string; title: string; class?: string } =
    $props();
</script>

<svg viewBox="0 0 24 24" class={className} role="img" aria-label={title}>
  <path d={path} fill="#{hex}" />
</svg>
