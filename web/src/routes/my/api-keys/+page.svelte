<script lang="ts">
  import { messages } from '$lib/components/ApiKeysView.messages';
  import ApiKeysView from '$lib/components/ApiKeysView.svelte';
  import ConnectedDevicesView from '$lib/components/ConnectedDevicesView.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  // The title shares the view's catalog rather than getting a second one — every
  // other string on this page is the view's.
  const s = $derived(t(messages, locale()));
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the content readable within the content column.
     Connected devices (MCP OAuth grants, freehire#3114) sits below API keys:
     both are credentials an account owner manages from one place. -->
<div class="flex max-w-3xl flex-col gap-10">
  <ApiKeysView />
  <ConnectedDevicesView />
</div>
