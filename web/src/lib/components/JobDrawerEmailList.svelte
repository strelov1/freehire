<script lang="ts">
  import { api, type EmailBody } from '#lib/api.js';
  import { avatarColor, avatarInitials } from '#lib/avatar.js';
  import StatusChip from '#lib/components/StatusChip.svelte';
  import { stageImplication, statusLabel } from '#lib/emailStatus.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { ApplicationEmail } from '#lib/types.js';
  import { timeAgo } from '#lib/utils.js';
  import { messages } from './JobDrawer.messages';

  // The mail linked to one application, in JobDrawer's Emails tab: one row per message, and
  // a click opens its body under it (fetched then, one open at a time).
  let { emails }: { emails: ApplicationEmail[] } = $props();

  const s = $derived(t(messages, locale()));

  let expandedId = $state<number | null>(null);
  let expandedBody = $state.raw<EmailBody | null>(null);
  let bodyLoading = $state(false);

  async function toggleEmail(id: number) {
    if (expandedId === id) {
      expandedId = null;
      expandedBody = null;
      return;
    }
    expandedId = id;
    expandedBody = null;
    bodyLoading = true;
    try {
      expandedBody = await api.getEmail(id);
    } catch {
      /* leave the row collapsed-open with no body; the list still shows */
    } finally {
      bodyLoading = false;
    }
  }
</script>

{#each emails as e (e.id)}
  <div class="overflow-hidden rounded-xl border border-border">
    <button
      type="button"
      onclick={() => toggleEmail(e.id)}
      aria-expanded={expandedId === e.id}
      class="flex w-full items-start gap-3 p-3 text-left transition-colors hover:bg-accent"
    >
      <div
        class="mt-0.5 flex size-9 shrink-0 select-none items-center justify-center rounded-full text-xs font-semibold text-white"
        style="background-color: {avatarColor(e.from_addr || e.from_name)}"
      >
        {avatarInitials(e.from_name, e.from_addr)}
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-baseline gap-2">
          <span class="min-w-0 flex-1 truncate text-sm font-medium"
            >{e.direction === 'outgoing'
              ? `${s.sent} · ${e.to_addrs || e.from_addr}`
              : e.from_name || e.from_addr}</span
          >
          <span class="shrink-0 text-[11px] text-muted-foreground">{timeAgo(e.received_at, locale())}</span>
        </div>
        <div class="mt-0.5 truncate text-sm text-muted-foreground">{e.subject || s.noSubject}</div>
        {#if e.direction !== 'outgoing' && statusLabel(e.status_signal, locale())}
          <!-- The chip, and what its signal means for the stage. The chip alone left
               three different situations looking identical: the signal moved the
               stage, it named one only the candidate may apply, or it was never
               about progress.
               Both run at the chip's own size (text-xs) rather than at a 10px the
               wrapper used to impose: StatusChip renders a Badge, which carries its
               size, so pinning the wrapper smaller would leave the explanation off
               the chip's baseline — and this thread has the room the dense inbox
               list does not. -->
          <span class="mt-1 inline-flex flex-wrap items-baseline gap-1.5 text-xs leading-4">
            <StatusChip signal={e.status_signal} />
            {#if stageImplication(e.status_signal, locale())}
              <span class="text-muted-foreground">{stageImplication(e.status_signal, locale())}</span>
            {/if}
          </span>
        {/if}
      </div>
    </button>
    {#if expandedId === e.id}
      <div class="border-t border-border p-3">
        {#if bodyLoading}
          <p class="text-sm text-muted-foreground">{s.loading}</p>
        {:else if expandedBody?.body_html}
          <!-- Untrusted sender HTML isolated in a sandboxed iframe (no scripts/forms/navigation). -->
          <iframe title={s.messageBodyTitle} sandbox="" srcdoc={expandedBody.body_html} class="h-96 w-full bg-white"
          ></iframe>
        {:else if expandedBody?.body_text}
          <pre
            class="max-h-96 overflow-y-auto whitespace-pre-wrap font-sans text-sm leading-relaxed">{expandedBody.body_text}</pre>
        {:else}
          <p class="text-sm text-muted-foreground">{s.noContent}</p>
        {/if}
      </div>
    {/if}
  </div>
{/each}
