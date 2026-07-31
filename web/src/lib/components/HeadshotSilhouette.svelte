<script lang="ts">
  // The stand-in shown wherever a headshot would go but none is stored: a head and
  // shoulders clipped by a square frame. It is the SVG twin of the `silhouette` helper the
  // photo-bearing Typst templates draw, so the profile, the CV preview, and the rendered
  // PDF agree about what "no photo yet" looks like.
  let { class: className = '' }: { class?: string } = $props();
</script>

<svg viewBox="0 0 100 100" role="presentation" class="bg-muted {className}">
  <circle cx="50" cy="36" r="18" class="fill-muted-foreground/35" />
  <ellipse cx="50" cy="94" rx="36" ry="25" class="fill-muted-foreground/35" />
</svg>
