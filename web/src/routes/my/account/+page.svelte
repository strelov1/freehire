<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import AccountLanguage from '#lib/components/AccountLanguage.svelte';
  import DeleteAccountButton from '#lib/components/DeleteAccountButton.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { Button } from '#lib/ui/index.js';
  import ChangeEmailDialog from './ChangeEmailDialog.svelte';
  import ChangePasswordDialog from './ChangePasswordDialog.svelte';
  import { messages } from './messages';

  // Email, password, language and sessions. Every action is cookie-only server-side: an API
  // key can neither change the email or password nor end a human's sessions. Language lives
  // here rather than in the profile because it is the account's, not the candidate's: it
  // switches the whole /my interface and the language the assistant answers in.
  //
  // The page itself is a list of what the account has, each with one button; the forms
  // live in dialogs, so the page reads as a summary rather than three open forms.

  const s = $derived(t(messages, locale()));
  const user = $derived(currentUser());
  // An OAuth-only account has no password to change; it sets one through the
  // forgotten-password flow, which proves the address instead. This has to come from
  // the server — being signed in says nothing about whether a password exists.
  const hasPassword = $derived(user?.has_password ?? false);

  // Which change just succeeded — the KEY, so a language switch re-renders it.
  let notice = $state<'emailChanged' | 'passwordChanged' | null>(null);

  let signingOut = $state(false);

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

<h1 class="mb-1 text-2xl max-lg:sr-only font-semibold tracking-tight">{s.title}</h1>
<p class="mb-6 text-sm text-muted-foreground max-lg:sr-only">{s.subtitle}</p>

<section class="mb-8 rounded-lg border border-border">
  <div class="flex flex-wrap items-center justify-between gap-3 p-4">
    <div class="min-w-0">
      <h2 class="text-sm font-semibold">{s.email.heading}</h2>
      <p class="truncate text-sm text-muted-foreground">{user?.email ?? ''}</p>
    </div>
    <ChangeEmailDialog onchanged={() => (notice = 'emailChanged')} />
  </div>

  <div class="flex flex-wrap items-center justify-between gap-3 border-t border-border p-4">
    <div class="min-w-0">
      <h2 class="text-sm font-semibold">{s.password.heading}</h2>
      {#if hasPassword}
        <p class="text-sm text-muted-foreground">{s.password.isSet}</p>
      {:else}
        <p class="max-w-prose text-sm text-muted-foreground">{s.password.noPasswordNotice}</p>
      {/if}
    </div>
    {#if hasPassword}
      <ChangePasswordDialog onchanged={() => (notice = 'passwordChanged')} />
    {/if}
  </div>

  {#if notice}
    <p role="status" class="border-t border-border p-4 text-sm text-muted-foreground">{s[notice]}</p>
  {/if}
</section>

<section class="mb-8 rounded-lg border border-border p-4">
  <AccountLanguage />
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
