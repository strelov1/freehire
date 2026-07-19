<script lang="ts">
  import CreditsView from '$lib/components/CreditsView.svelte';
</script>

<svelte:head>
  <title>Credits — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the balance and history readable within the content column. -->
<div class="max-w-3xl">
  <CreditsView />
</div>
