<script lang="ts">
  import { messages } from '$lib/components/PlanView.messages';
  import PlanView from '$lib/components/PlanView.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  // The title shares the view's catalog rather than getting a second one — every
  // other string on this page is the view's.
  const s = $derived(t(messages, locale()));
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the allowances and history readable within the content column. -->
<div class="max-w-3xl">
  <PlanView />
</div>
