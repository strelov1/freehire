<script lang="ts">
  import ReminderSettings from '$lib/components/ReminderSettings.svelte';
  import SavedJobs from '$lib/components/SavedJobs.svelte';
</script>

<svelte:head>
  <title>Saved — Activity — freehire</title>
</svelte:head>

<div class="flex flex-col gap-4">
  <ReminderSettings />
  <SavedJobs />
</div>
