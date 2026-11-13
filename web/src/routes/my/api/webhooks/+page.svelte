<script lang="ts">
  import WebhookSettingsView from '#lib/components/WebhookSettingsView.svelte';
  import { messages } from '#lib/components/WebhookSettingsView.messages.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  // The same webhook as Integrations → Webhook; the plan strip is the workspace layout's.
  const s = $derived(t(messages, locale()));
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<WebhookSettingsView />
