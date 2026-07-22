<script lang="ts">
  import { resolve } from '$app/paths';
  import { bannerVisible, deny, grant, trackersAllowed } from '$lib/consent.svelte';
  import { startTrackers } from '$lib/trackers';

  // Accept records consent and starts the trackers immediately; Reject records the
  // refusal and starts nothing. The two are deliberately equal in prominence — no
  // pre-selected default — so the choice is freely given (ePrivacy/GDPR).
  function accept() {
    grant();
    startTrackers();
  }

  // Reject records the refusal. If trackers were already running this session (a
  // withdrawal via the footer re-open), reload so GA/PostHog actually stop — they
  // cannot be fully torn down in place. A first-time rejection starts nothing, so
  // no reload is needed.
  function reject() {
    const wasRunning = trackersAllowed();
    deny();
    if (wasRunning) location.reload();
  }
</script>

{#if bannerVisible()}
  <div
    role="dialog"
    aria-modal="false"
    aria-label="Cookie consent"
    class="fixed inset-x-4 bottom-4 z-50 rounded-lg border border-border bg-background/95 px-4 py-3 shadow-lg backdrop-blur sm:inset-x-auto sm:left-4 sm:max-w-md"
  >
    <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
      <p class="font-mono text-sm font-semibold text-foreground">Cookies?</p>
      <p class="min-w-0 flex-1 text-xs leading-relaxed text-muted-foreground">
        We use Google Analytics and PostHog to understand usage.
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal route via resolve() -->
        <a
          href={resolve('/privacy')}
          class="font-mono underline underline-offset-2 transition-colors hover:text-foreground"
        >
          Learn more
        </a>
      </p>
      <div class="flex shrink-0 gap-2">
        <button
          type="button"
          onclick={accept}
          class="rounded border border-border px-3 py-1 font-mono text-sm text-foreground transition-colors hover:bg-muted"
        >
          Accept
        </button>
        <button
          type="button"
          onclick={reject}
          class="rounded border border-border px-3 py-1 font-mono text-sm text-foreground transition-colors hover:bg-muted"
        >
          Reject
        </button>
      </div>
    </div>
  </div>
{/if}
