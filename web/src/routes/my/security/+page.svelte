<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { currentUser } from '$lib/auth.svelte';
  import { Button } from '$lib/ui';

  // Password + sessions. Both actions are cookie-only server-side: an API key can
  // neither change the password nor end a human's sessions.

  const user = $derived(currentUser());
  // An OAuth-only account has no password to change; it sets one through the
  // forgotten-password flow, which proves the address instead. This has to come from
  // the server — being signed in says nothing about whether a password exists.
  const hasPassword = $derived(user?.has_password ?? false);

  let currentPassword = $state('');
  let password = $state('');
  let confirmation = $state('');
  let saving = $state(false);
  let changeError = $state<string | null>(null);
  let changed = $state(false);

  let signingOut = $state(false);

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 401) return 'That current password is not right.';
      if (e.status === 400) return 'Choose a password of 8–72 characters.';
    }
    return 'Something went wrong. Please try again.';
  }

  async function changePassword(e: SubmitEvent) {
    e.preventDefault();
    changeError = null;
    changed = false;
    if (password !== confirmation) {
      changeError = 'The two new passwords do not match.';
      return;
    }
    saving = true;
    try {
      await api.reauthenticatePassword(currentPassword);
      await api.changePassword(currentPassword, password);
      changed = true;
      currentPassword = password = confirmation = '';
    } catch (err) {
      changeError = messageFor(err);
    } finally {
      saving = false;
    }
  }

  // Signing out everywhere ends this session too, so the app must return to a
  // signed-out state rather than keep rendering an account page.
  async function signOutEverywhere() {
    signingOut = true;
    try {
      await api.logoutEverywhere();
    } catch {
      // The cookie may already be gone; fall through to the same re-resolve.
    }
    await invalidateAll();
    await goto(resolve('/'));
  }
</script>

<svelte:head><title>Security · freehire</title></svelte:head>

<h1 class="mb-1 text-lg font-semibold tracking-tight">Security</h1>
<p class="mb-6 text-sm text-muted-foreground">
  Change your password and end sessions on other devices.
</p>

<section class="mb-8 rounded-lg border border-border p-4">
  <h2 class="mb-1 text-sm font-semibold">Password</h2>
  <p class="mb-4 text-sm text-muted-foreground">
    Changing it signs out every other device. You stay signed in here.
  </p>

  {#if !hasPassword}
    <p class="text-sm text-muted-foreground">
      This account signs in with a provider and has no password. To add one — a second way
      in, independent of the provider — sign out, choose “Forgot your password?” on the
      sign-in screen, and set it with the code we email you.
    </p>
  {:else}
    <form class="flex max-w-sm flex-col gap-3" onsubmit={changePassword}>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Current password</span>
        <input
          type="password"
          bind:value={currentPassword}
          required
          autocomplete="current-password"
          class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">New password</span>
        <input
          type="password"
          bind:value={password}
          required
          minlength={8}
          maxlength={72}
          autocomplete="new-password"
          class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Repeat new password</span>
        <input
          type="password"
          bind:value={confirmation}
          required
          autocomplete="new-password"
          class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>

      {#if changeError}
        <p class="text-sm text-destructive">{changeError}</p>
      {/if}
      {#if changed}
        <p class="text-sm text-muted-foreground">Password changed. Other devices were signed out.</p>
      {/if}

      <Button type="submit" variant="primary" disabled={saving} class="mt-1">
        {saving ? 'Saving…' : 'Change password'}
      </Button>
    </form>
  {/if}
</section>

<section class="rounded-lg border border-border p-4">
  <h2 class="mb-1 text-sm font-semibold">Sessions</h2>
  <p class="mb-4 text-sm text-muted-foreground">
    Sign out of every device, including this one. Use this if you think someone else has
    access. Your API keys keep working — revoke those individually.
  </p>
  <Button variant="outline" disabled={signingOut} onclick={signOutEverywhere}>
    {signingOut ? 'Signing out…' : 'Sign out everywhere'}
  </Button>
</section>
