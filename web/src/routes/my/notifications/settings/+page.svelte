<script lang="ts">
  import EmailGroupSettings from '#lib/components/EmailGroupSettings.svelte';
  import ReminderSettings from '#lib/components/ReminderSettings.svelte';
</script>

<svelte:head>
  <title>Notification settings — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex.
     Full column width, like every other section, so card edges do not move between pages.

     Two cards, not one: ReminderSettings owns the activity group (reminders,
     follow-ups, report outcomes) and its delivery channels, and EmailGroupSettings
     owns the other two email groups. Between them they cover exactly what the public
     /unsubscribe page offers, so signing in never means finding fewer controls than
     a link in an email gave you. -->
<div class="space-y-4">
  <ReminderSettings />
  <EmailGroupSettings />
</div>
