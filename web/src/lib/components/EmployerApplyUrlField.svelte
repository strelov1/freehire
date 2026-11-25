<script lang="ts">
  // The Apply URL field with its "Fill in from this link" button. The link is read into
  // the empty fields only (the parent decides how, in `onResult`), so it never overwrites
  // what was typed. A vacancy already in the catalogue is a good answer here, unlike on
  // /submit. Best-effort: a failure leaves the form as it was. The field lives only while
  // the create form is open, so closing the form discards a request still in flight.
  import { api } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { prefillFound } from '#lib/prefill.js';
  import type { PrefillResult } from '#lib/types.js';
  import { Button, Input } from '#lib/ui/index.js';
  import { messages } from './EmployerView.messages';

  let { url = $bindable(''), onResult }: { url?: string; onResult: (result: PrefillResult) => void } = $props();

  const s = $derived(t(messages, locale()));
  let prefilling = $state(false);
  let missURL = $state<string | null>(null);
  const miss = $derived(missURL !== null && missURL === url.trim());
  let alive = true;
  $effect(() => () => (alive = false));

  async function prefill() {
    const target = url.trim();
    if (!target || prefilling) return;
    prefilling = true;
    missURL = null;
    try {
      const result = await api.prefillEmployerVacancy(target);
      // The form closed, or the URL was edited, while the request was out.
      if (!alive || url.trim() !== target) return;
      if (prefillFound(result)) onResult(result);
      else missURL = target;
    } catch {
      // Best-effort: leave the form exactly as it was.
    } finally {
      prefilling = false;
    }
  }
</script>

<div class="flex gap-2">
  <Input bind:value={url} type="url" placeholder="https://…" class="w-full" />
  <Button type="button" variant="outline" class="shrink-0" disabled={prefilling || !url.trim()} onclick={prefill}>
    {prefilling ? s.prefillBusy : s.prefillButton}
  </Button>
</div>
{#if miss}
  <span class="text-xs text-muted-foreground" role="status">{s.prefillMiss}</span>
{/if}
