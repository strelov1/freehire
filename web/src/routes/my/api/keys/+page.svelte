<script lang="ts">
  import ApiKeysView from '#lib/components/ApiKeysView.svelte';
  import ConnectedDevicesView from '#lib/components/ConnectedDevicesView.svelte';
  import { messages } from '#lib/components/ApiKeysView.messages.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  // The same views as Integrations → API keys; the plan strip is the workspace layout's.
  const s = $derived(t(messages, locale()));
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<div class="flex flex-col gap-10">
  <ApiKeysView returnTo="/my/api/keys" />
  <ConnectedDevicesView />
</div>
