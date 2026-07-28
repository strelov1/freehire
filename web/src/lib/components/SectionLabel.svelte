<script lang="ts">
  // The mono `// section` label that heads every section on the landing pages
  // (home/about, contribute, the feature landings). The `//` is part of the
  // label, so callers pass the bare section name.
  let {
    text,
    class: className = '',
    style = '',
  }: { text: string; class?: string; style?: string } = $props();
</script>

<p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground {className}" {style}>
  // {text}
</p>
