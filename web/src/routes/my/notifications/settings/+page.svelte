<script lang="ts">
  import ReminderSettings from '$lib/components/ReminderSettings.svelte';
</script>

<svelte:head>
  <title>Notification settings — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the settings card readable within the content column. -->
<div class="max-w-3xl">
  <ReminderSettings />
</div>
