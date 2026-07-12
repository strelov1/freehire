<script lang="ts">
  import MySubmissionsView from '$lib/components/MySubmissionsView.svelte';
</script>

<svelte:head>
  <title>My submissions — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the list readable within the content column. -->
<div class="max-w-3xl">
  <MySubmissionsView />
</div>
