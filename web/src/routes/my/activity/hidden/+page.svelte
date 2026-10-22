<script lang="ts">
  import { messages } from '#lib/components/activity.messages.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import Hidden from '#lib/components/Hidden.svelte';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitles.hidden}</title>
</svelte:head>

<Hidden />
