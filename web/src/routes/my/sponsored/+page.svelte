<script lang="ts">
  import { messages } from '#lib/components/Sponsored.messages.js';
  import SponsoredCampaigns from '#lib/components/SponsoredCampaigns.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head><title>{s.navTitle} — freehire</title></svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<SponsoredCampaigns />
