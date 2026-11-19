<script lang="ts">
  import { api, isPlanRefusal } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { browserTimezone } from '#lib/mentorship.js';
  import { Button } from '#lib/ui/index.js';
  import PlanLimitLink from './PlanLimitLink.svelte';
  import { messages } from './JobDrawer.messages';

  // The application's booking link: turned on here, then shown with a copy button. The
  // zone sent is the browser's; the default hours are wall-clock times in it. `url` is
  // bound up to the drawer so a link turned on here survives a tab switch.
  let { applicationId, url = $bindable() }: { applicationId: number; url: string | null } = $props();

  const s = $derived(t(messages, locale()).bookingLink);
  let busy = $state(false);
  let needsPlan = $state(false);
  let failed = $state(false);
  let copied = $state(false);

  async function turnOn() {
    busy = true;
    needsPlan = failed = false;
    try {
      url = await api.enableBookingLink(applicationId, browserTimezone());
    } catch (e) {
      needsPlan = isPlanRefusal(e);
      failed = !needsPlan;
    } finally {
      busy = false;
    }
  }

  async function copy() {
    if (!url) return;
    try {
      await navigator.clipboard.writeText(url);
    } catch {
      return; // no clipboard permission: the URL stays selectable in the field
    }
    copied = true;
    setTimeout(() => (copied = false), 2000);
  }
</script>

<section class="flex flex-col gap-2 text-sm">
  <h3 class="font-medium">{s.title}</h3>
  <p class="text-muted-foreground">{s.hint}</p>
  {#if url}
    <div class="flex items-center gap-2">
      <input
        readonly
        value={url}
        aria-label={s.title}
        class="border-input bg-background min-w-0 flex-1 rounded-md border px-3 py-1.5 text-sm"
      />
      <Button variant="outline" size="sm" onclick={copy}>{copied ? s.copied : s.copy}</Button>
    </div>
  {:else}
    <div>
      <Button variant="outline" size="sm" disabled={busy} onclick={turnOn}>{s.turnOn}</Button>
    </div>
  {/if}
  {#if needsPlan}
    <div class="flex flex-wrap items-center gap-3">
      <p class="text-muted-foreground">{s.needsPlan}</p>
      <PlanLimitLink />
    </div>
  {:else if failed}
    <p class="text-destructive">{s.failed}</p>
  {/if}
</section>
