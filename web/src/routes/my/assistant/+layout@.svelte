<script lang="ts">
  // The agent is an app-like, full-width surface. The `@` resets it past the /my layout
  // (account-nav rail + max-w-6xl), so it renders directly in the root layout — full width,
  // with just the top bar above it. The footer is already hidden for /my/* in the root layout.
  let { children } = $props();
</script>

{@render children()}
