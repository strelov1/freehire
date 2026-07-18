<script lang="ts">
  import { currentUser } from '$lib/auth.svelte';
  import CvList from '$lib/components/cv/CvList.svelte';

  // Beta-tester-gated in the UI (the server still admits moderators via RequireModeratorOrBeta
  // as an admin override). This just keeps a non-beta user who navigates here directly from
  // hitting a raw 403.
  const eligible = $derived(currentUser()?.beta_tester === true);
</script>

<svelte:head>
  <title>CV builder — freehire</title>
</svelte:head>

<div class="max-w-3xl">
  {#if eligible}
    <CvList />
  {:else}
    <p class="text-muted-foreground">The CV builder is in beta and not available on your account yet.</p>
  {/if}
</div>
