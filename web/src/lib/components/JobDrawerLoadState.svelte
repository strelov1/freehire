<script lang="ts">
  import { Button } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './JobDrawer.messages.js';

  let {
    loading,
    error,
    onretry,
  }: {
    loading: boolean;
    error: string | null;
    onretry: () => void;
  } = $props();
  const s = $derived(t(messages, locale()));
</script>

{#if loading}
  <p class="text-sm text-muted-foreground" role="status">{s.loading}</p>
{:else if error}
  <p class="text-sm text-destructive" role="alert">{error}</p>
  <Button variant="outline" size="sm" onclick={onretry}>{s.retryHistory}</Button>
{/if}
