<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import { announce } from '#lib/announce.svelte.js';
  import { api, ApiError } from '#lib/api.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import { Button } from '#lib/ui/index.js';

  // Prompts a signed-in user who has never proven their address to confirm it with the
  // mailed six-digit code. Self-gating: it renders nothing for signed-out visitors and
  // for verified accounts, so the layout can mount it unconditionally.
  //
  // An unverified account is fully usable — this is a prompt, not a wall. What it buys is
  // that an unverified account cannot be silently joined by an OAuth identity for the same
  // address (the server seizes it instead), so confirming here keeps the account the
  // user's own if they later sign in with a provider.

  const user = $derived(currentUser());
  const needsVerification = $derived(user !== null && !user.email_verified);

  let open = $state(false);
  let code = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);
  let sent = $state(false);

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 400) return 'That code is not valid or has expired. Send a new one.';
      if (e.status === 429) return 'A code was just sent — check your inbox.';
      if (e.status === 503) return 'Email delivery is unavailable right now.';
      if (e.status === 409) return 'This email is already confirmed.';
    }
    return 'Something went wrong. Please try again.';
  }

  async function sendCode() {
    busy = true;
    error = null;
    try {
      await api.requestEmailVerification();
      sent = true;
      open = true;
      announce('We sent a code to your inbox. It expires in 15 minutes.');
    } catch (e) {
      error = messageFor(e);
      announce(error, 'assertive');
      open = true;
    } finally {
      busy = false;
    }
  }

  async function confirm(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = null;
    try {
      await api.confirmEmailVerification(code.trim());
      // Re-resolve the user so the banner disappears on the spot.
      await invalidateAll();
      announce('Email address confirmed');
    } catch (err) {
      error = messageFor(err);
      announce(error, 'assertive');
    } finally {
      busy = false;
    }
  }
</script>

{#if needsVerification}
  <div class="border-b border-border bg-secondary/50">
    <!-- One row on a phone: the short sentence and the button side by side. The full
         sentence with the address wrapped to three lines there and put the button on a
         fourth, in front of every page of a fresh account. The address stays, truncated —
         without it nobody with two inboxes knows where the code is going. -->
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-3 px-4 py-2 text-sm sm:py-3">
      <div class="flex items-center justify-between gap-3 sm:flex-wrap sm:justify-start">
        <p class="min-w-0 text-muted-foreground">
          <span class="flex min-w-0 gap-1 sm:hidden">
            <span class="shrink-0">Confirm</span>
            <span class="truncate font-medium text-foreground">{user?.email}</span>
          </span>
          <span class="hidden sm:inline">
            Confirm <span class="font-medium text-foreground">{user?.email}</span> to secure your account.
          </span>
        </p>
        {#if !open}
          <Button variant="outline" size="sm" class="shrink-0" disabled={busy} onclick={sendCode}>
            {busy ? 'Sending…' : 'Send code'}
          </Button>
        {/if}
      </div>

      {#if open}
        <form class="flex flex-wrap items-end gap-2" onsubmit={confirm}>
          <label class="flex flex-col gap-1">
            <span class="text-xs text-muted-foreground">Six-digit code</span>
            <input
              bind:value={code}
              inputmode="numeric"
              autocomplete="one-time-code"
              maxlength={6}
              required
              class="w-32 rounded-md border border-border bg-background px-3 py-2 tracking-widest focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? 'Checking…' : 'Confirm'}
          </Button>
          <Button variant="ghost" disabled={busy} onclick={sendCode}>Resend</Button>
        </form>
      {/if}

      {#if error}
        <p class="text-destructive">{error}</p>
      {:else if sent && open}
        <p class="text-muted-foreground">We sent a code to your inbox. It expires in 15 minutes.</p>
      {/if}
    </div>
  </div>
{/if}
