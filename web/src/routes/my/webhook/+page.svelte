<script lang="ts">
  import WebhookSettingsView from '$lib/components/WebhookSettingsView.svelte';
</script>

<svelte:head>
  <title>Webhook — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the content readable within the content column. -->
<div class="max-w-3xl">
  <WebhookSettingsView />
</div>
