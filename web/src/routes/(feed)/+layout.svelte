<script lang="ts">
  // Placeholder: groups `/` and `/jobs/[slug]` under one URL-invisible route group so a
  // later step can add a shared list column here without duplicating either route's
  // load logic. Passthrough only for now — no visual change from this step.
  let { children } = $props();
</script>

{@render children()}
