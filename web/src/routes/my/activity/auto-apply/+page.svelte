<script lang="ts">
  import AutoApplyList from '#lib/components/AutoApplyList.svelte';
  import { messages } from '#lib/components/activity.messages.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  const s = $derived(t(messages, locale()));
</script>

<svelte:head>
  <title>{s.headTitles.autoApply}</title>
</svelte:head>

<AutoApplyList />
