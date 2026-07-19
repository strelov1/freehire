<script lang="ts">
  import ReferralsView from '$lib/components/ReferralsView.svelte';
</script>

<svelte:head>
  <title>Referrals — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<div class="max-w-3xl">
  <h1 class="mb-4 text-lg font-semibold tracking-tight">Referrals</h1>
  <ReferralsView />
</div>
