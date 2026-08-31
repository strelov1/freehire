<script lang="ts">
  import PlanView from '$lib/components/PlanView.svelte';
</script>

<svelte:head>
  <title>Your plan — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the allowances and history readable within the content column. -->
<div class="max-w-3xl">
  <PlanView />
</div>
