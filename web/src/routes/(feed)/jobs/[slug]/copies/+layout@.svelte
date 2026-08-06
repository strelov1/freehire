<script lang="ts">
  // This sub-page stays full-page, not wrapped in the (feed) list column — the `@`
  // resets it past that layout, back to root, same convention as my/assistant/+layout@.svelte.
  let { children } = $props();
</script>

{@render children()}
