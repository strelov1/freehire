<script lang="ts">
  import { resolve } from '$app/paths';
  import { bannerVisible, deny, grant, trackersAllowed } from '#lib/consent.svelte.js';
  import { startTrackers } from '#lib/trackers.js';

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
    class="fixed inset-x-4 bottom-safe z-50 rounded-xl border border-border bg-background p-4 sm:inset-x-auto sm:right-4 sm:w-96"
  >
    <div class="flex flex-col gap-3">
      <p class="text-sm font-semibold text-foreground">Cookies?</p>
      <p class="text-sm leading-relaxed text-muted-foreground">
        We use Google Analytics and PostHog to understand usage.
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal route via resolve() -->
        <a
          href={resolve('/privacy')}
          class="underline underline-offset-2 transition-colors hover:text-foreground"
        >
          Learn more
        </a>
      </p>
      <div class="flex justify-end gap-2">
        <button
          type="button"
          onclick={accept}
          class="flex min-h-9 items-center justify-center rounded-md border border-border px-4 text-sm font-medium text-foreground transition-colors hover:bg-muted"
        >
          Accept
        </button>
        <button
          type="button"
          onclick={reject}
          class="flex min-h-9 items-center justify-center rounded-md border border-border px-4 text-sm font-medium text-foreground transition-colors hover:bg-muted"
        >
          Reject
        </button>
      </div>
    </div>
  </div>
{/if}
