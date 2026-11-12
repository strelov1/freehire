<script lang="ts">
  import AccountPageHeading from '#lib/components/AccountPageHeading.svelte';
  import { messages } from '#lib/components/ReferralsView.messages.js';
  import ReferralsView from '#lib/components/ReferralsView.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  // The title and heading share the view's catalog rather than getting a second
  // one — every other string on this page is the view's.
  const s = $derived(t(messages, locale()));
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<div>
  <AccountPageHeading class="mb-4">{s.tabs.offers}</AccountPageHeading>
  <ReferralsView tab="offers" />
</div>
