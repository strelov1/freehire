<script lang="ts">
  import { resolve } from '$app/paths';
  import ReminderSettings from '$lib/components/ReminderSettings.svelte';
</script>

<svelte:head>
  <title>Notifications — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the settings card readable within the content column. -->
<div class="max-w-3xl">
  <ReminderSettings />
  <p class="mt-4 text-sm text-muted-foreground">
    Looking for what's already been sent?
    <a
      href={resolve('/my/notifications/history')}
      class="font-medium text-foreground underline underline-offset-2 hover:opacity-80"
    >
      View notification history
    </a>
  </p>
</div>
