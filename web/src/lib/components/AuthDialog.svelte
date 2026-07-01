<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { login, register } from '$lib/auth.svelte';
  import { authDialog } from '$lib/auth-dialog.svelte';
  import { ApiError, oauthProviders } from '$lib/api';
  import { Button } from '$lib/ui';
  import ProviderIcon from './ProviderIcon.svelte';

  // `mode` is bindable so the in-dialog toggle can switch between sign in and
  // register without the parent re-opening it. `initialError` lets the layout
  // surface a failed OAuth callback (the ?auth_error redirect) in the dialog.
  let {
    mode = $bindable(),
    onClose,
    initialError = null,
  }: { mode: 'login' | 'register'; onClose: () => void; initialError?: string | null } = $props();

  let email = $state('');
  let password = $state('');
  // The initial capture is deliberate: the dialog is recreated on every open
  // (it renders under {#if}), so the seed error never goes stale.
  // svelte-ignore state_referenced_locally
  let error = $state<string | null>(initialError);
  let submitting = $state(false);

  const providerLabels: Record<string, string> = {
    google: 'Google',
    github: 'GitHub',
    linkedin: 'LinkedIn',
  };

  // Enabled OAuth providers; an unreachable endpoint just means no provider
  // buttons — the email/password form must keep working either way.
  let providers = $state<string[]>([]);
  oauthProviders()
    .then((names) => {
      providers = names.filter((n) => n in providerLabels);
    })
    .catch(() => {});

  const title = $derived(mode === 'login' ? 'Sign in' : 'Create account');

  // Where to go after sign-in. When a guarded page bounced the user here to sign
  // in (e.g. a shared /jobs/swipe?filter link), `redirectTo` is that deep link;
  // otherwise stay on the current page (in-place prompts like a job's Save
  // button). OAuth passes this to the backend, which echoes it back sanitized;
  // password login navigates to it here after success.
  const returnTo = $derived(authDialog.redirectTo ?? page.url.pathname + page.url.search);

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 401) return 'Invalid email or password.';
      if (e.status === 409) return 'That email is already registered.';
      if (e.status === 400) return 'Enter a valid email and a password of at least 8 characters.';
    }
    return 'Something went wrong. Please try again.';
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = null;
    submitting = true;
    // Capture before onClose(), which clears the dialog's redirectTo.
    const target = returnTo;
    try {
      await (mode === 'login' ? login : register)(email, password);
      onClose();
      if (target !== page.url.pathname + page.url.search) {
        // eslint-disable-next-line svelte/no-navigation-without-resolve -- a validated same-origin path from the guard, not a typed route
        await goto(target);
      }
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }

  function toggleMode() {
    mode = mode === 'login' ? 'register' : 'login';
    error = null;
  }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onClose()} />

<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
  <!-- Backdrop is a real button so closing on click is keyboard-accessible. -->
  <button type="button" aria-label="Close dialog" class="absolute inset-0 bg-black/50" onclick={onClose}
  ></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-label={title}
    class="relative w-full max-w-sm rounded-lg border border-border bg-background p-6 shadow-lg"
  >
    <h2 class="mb-4 text-base font-semibold tracking-tight">{title}</h2>

    {#if providers.length > 0}
      <div class="mb-4 flex flex-col gap-2">
        {#each providers as provider (provider)}
          <Button
            variant="outline"
            href={`/api/v1/auth/oauth/${provider}/start?returnTo=${encodeURIComponent(returnTo)}`}
          >
            <ProviderIcon {provider} />
            Continue with {providerLabels[provider]}
          </Button>
        {/each}
      </div>

      <div class="mb-4 flex items-center gap-3 text-xs text-muted-foreground">
        <span class="h-px flex-1 bg-border"></span>
        or
        <span class="h-px flex-1 bg-border"></span>
      </div>
    {/if}

    <form class="flex flex-col gap-3" onsubmit={submit}>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Email</span>
        <input
          type="email"
          bind:value={email}
          required
          autocomplete="email"
          class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>

      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Password</span>
        <input
          type="password"
          bind:value={password}
          required
          minlength={mode === 'register' ? 8 : undefined}
          autocomplete={mode === 'login' ? 'current-password' : 'new-password'}
          class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>

      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}

      <Button type="submit" variant="primary" disabled={submitting} class="mt-1">
        {submitting ? 'Please wait…' : title}
      </Button>
    </form>

    <p class="mt-4 text-center text-sm text-muted-foreground">
      {mode === 'login' ? 'No account?' : 'Already have an account?'}
      <button
        type="button"
        onclick={toggleMode}
        class="font-medium text-foreground underline-offset-4 hover:underline"
      >
        {mode === 'login' ? 'Create one' : 'Sign in'}
      </button>
    </p>
  </div>
</div>
