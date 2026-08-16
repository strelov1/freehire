<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { currentUser } from '$lib/auth.svelte';
  import DeleteAccountButton from '$lib/components/DeleteAccountButton.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import { Button } from '$lib/ui';
  import { messages } from './messages';

  // Password + sessions. Both actions are cookie-only server-side: an API key can
  // neither change the password nor end a human's sessions.

  const s = $derived(t(messages, locale()));
  const user = $derived(currentUser());
  // An OAuth-only account has no password to change; it sets one through the
  // forgotten-password flow, which proves the address instead. This has to come from
  // the server — being signed in says nothing about whether a password exists.
  const hasPassword = $derived(user?.has_password ?? false);

  let currentPassword = $state('');
  let password = $state('');
  let confirmation = $state('');
  let saving = $state(false);
  // Stores which message to show, not the resolved string — so a language switch
  // while an error is visible re-renders it in the new language instead of
  // leaving behind a snapshot from whichever locale was active when it occurred.
  type PasswordErrorKey = 'mismatchError' | 'wrongCurrentPassword' | 'weakPassword' | 'genericError';
  let changeErrorKey = $state<PasswordErrorKey | null>(null);
  let changed = $state(false);

  let signingOut = $state(false);

  function keyFor(e: unknown): PasswordErrorKey {
    if (e instanceof ApiError) {
      if (e.status === 401) return 'wrongCurrentPassword';
      if (e.status === 400) return 'weakPassword';
    }
    return 'genericError';
  }

  async function changePassword(e: SubmitEvent) {
    e.preventDefault();
    changeErrorKey = null;
    changed = false;
    if (password !== confirmation) {
      changeErrorKey = 'mismatchError';
      return;
    }
    saving = true;
    try {
      await api.reauthenticatePassword(currentPassword);
      await api.changePassword(currentPassword, password);
      changed = true;
      currentPassword = password = confirmation = '';
    } catch (err) {
      changeErrorKey = keyFor(err);
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

<svelte:head><title>{s.headTitle}</title></svelte:head>

<h1 class="mb-1 text-lg font-semibold tracking-tight">{s.title}</h1>
<p class="mb-6 text-sm text-muted-foreground">{s.subtitle}</p>

<section class="mb-8 rounded-lg border border-border p-4">
  <h2 class="mb-1 text-sm font-semibold">{s.password.heading}</h2>
  <p class="mb-4 text-sm text-muted-foreground">{s.password.subheading}</p>

  {#if !hasPassword}
    <p class="text-sm text-muted-foreground">{s.password.noPasswordNotice}</p>
  {:else}
    <form class="flex max-w-sm flex-col gap-3" onsubmit={changePassword}>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">{s.password.currentPasswordLabel}</span>
        <input
          type="password"
          bind:value={currentPassword}
          required
          autocomplete="current-password"
          class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">{s.password.newPasswordLabel}</span>
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
        <span class="text-muted-foreground">{s.password.repeatPasswordLabel}</span>
        <input
          type="password"
          bind:value={confirmation}
          required
          autocomplete="new-password"
          class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>

      {#if changeErrorKey}
        <p class="text-sm text-destructive">{s.password[changeErrorKey]}</p>
      {/if}
      {#if changed}
        <p class="text-sm text-muted-foreground">{s.password.changed}</p>
      {/if}

      <Button type="submit" variant="primary" disabled={saving} class="mt-1">
        {saving ? s.password.saving : s.password.save}
      </Button>
    </form>
  {/if}
</section>

<section class="rounded-lg border border-border p-4">
  <h2 class="mb-1 text-sm font-semibold">{s.sessions.heading}</h2>
  <p class="mb-4 text-sm text-muted-foreground">{s.sessions.subheading}</p>
  <Button variant="outline" disabled={signingOut} onclick={signOutEverywhere}>
    {signingOut ? s.sessions.signingOut : s.sessions.signOut}
  </Button>
</section>

<section class="mt-8 rounded-lg border border-destructive/30 p-4">
  <h2 class="mb-1 text-sm font-semibold">{s.dangerZone.heading}</h2>
  <p class="mb-4 text-sm text-muted-foreground">{s.dangerZone.description}</p>
  <DeleteAccountButton />
</section>
