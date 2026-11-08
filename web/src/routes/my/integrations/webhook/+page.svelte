<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from '#lib/components/WebhookSettingsView.messages.js';
  import WebhookSettingsView from '#lib/components/WebhookSettingsView.svelte';

  // The title shares the view's catalog rather than getting a second one — every
  // other string on this page is the view's.
  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitle}</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex.
     Full column width, like every other section, so card edges do not move between pages. -->
<div>
  <WebhookSettingsView />
</div>
