<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { login, register } from '$lib/auth.svelte';
  import { authDialog } from '$lib/auth-dialog.svelte';
  import { api, ApiError } from '$lib/api';
  import { Button, Dialog } from '$lib/ui';
  import { ProviderIcon } from '$lib/ui';

  // `mode` is bindable so the in-dialog toggle can switch between sign in and
  // register without the parent re-opening it. `initialError` lets the layout
  // surface a failed OAuth callback (the ?auth_error redirect) in the dialog.
  let {
    mode = $bindable(),
    onClose,
    initialError = null,
  }: {
    mode: 'login' | 'register' | 'forgot' | 'reset';
    onClose: () => void;
    initialError?: string | null;
  } = $props();

  // The parent mounts this component to open it and unmounts on onClose, so the
  // dialog is open for its whole life. Dialog owns the closing — Escape, the
  // backdrop and its own button all land on `open`, and this reports it upward
  // rather than each affordance calling onClose itself.
  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  let email = $state('');
  let password = $state('');
  // Recovery: the mailed six-digit code, and a note shown between the two steps.
  let code = $state('');
  let notice = $state<string | null>(null);
  // The initial capture is deliberate: the dialog is recreated on every open
  // (it renders under {#if}), so the seed error never goes stale.
  // svelte-ignore state_referenced_locally
  let error = $state<string | null>(initialError);
  let submitting = $state(false);

  const providerLabels: Record<string, string> = {
    google: 'Google',
    github: 'GitHub',
    linkedin: 'LinkedIn',
    apple: 'Apple',
  };

  // Enabled OAuth providers; an unreachable endpoint just means no provider
  // buttons — the email/password form must keep working either way.
  let providers = $state<string[]>([]);
  api.oauthProviders()
    .then((names) => {
      providers = names.filter((n) => n in providerLabels);
    })
    .catch(() => {});

  const titles = {
    login: 'Sign in',
    register: 'Create account',
    forgot: 'Reset your password',
    reset: 'Set a new password',
  } as const;
  const title = $derived(titles[mode]);
  // The provider buttons and the sign-in/register toggle belong to the credential
  // modes only; the recovery steps are a linear flow.
  const isRecovery = $derived(mode === 'forgot' || mode === 'reset');

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
      if (e.status === 429) return 'A code was just sent — check your inbox.';
      if (e.status === 503) return 'Email delivery is unavailable right now.';
      if (e.status === 400) {
        return isRecovery
          ? 'That code is not valid or has expired, or the password is too short.'
          : 'Enter a valid email and a password of at least 8 characters.';
      }
    }
    return 'Something went wrong. Please try again.';
  }

  // Ask for a reset code. The server answers the same way whether or not the address
  // has an account, so the UI must not imply that a code is on its way to a real one.
  async function requestReset() {
    error = null;
    submitting = true;
    try {
      await api.forgotPassword(email);
      notice = 'If that address has an account, a code is on its way. It expires in 15 minutes.';
      mode = 'reset';
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }

  // Set the new password, then sign in with it: the reset revokes every session, so
  // there is nothing to carry over.
  async function completeReset() {
    error = null;
    submitting = true;
    try {
      await api.resetPassword(email, code.trim(), password);
      await login(email, password);
      onClose();
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (mode === 'forgot') return requestReset();
    if (mode === 'reset') return completeReset();
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

  // Every mode change clears whatever the previous step was saying, so a stale error
  // or notice never bleeds into the next screen.
  function switchMode(next: typeof mode) {
    mode = next;
    error = null;
    notice = null;
  }
</script>

<Dialog bind:open {title} class="sm:max-w-sm">

    {#if providers.length > 0 && !isRecovery}
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

      {#if mode === 'reset'}
        <label class="flex flex-col gap-1 text-sm">
          <span class="text-muted-foreground">Code from the email</span>
          <input
            bind:value={code}
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength={6}
            required
            class="rounded-md border border-border bg-background px-3 py-2 tracking-widest focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </label>
      {/if}

      {#if mode !== 'forgot'}
        <label class="flex flex-col gap-1 text-sm">
          <span class="text-muted-foreground">{mode === 'reset' ? 'New password' : 'Password'}</span>
          <input
            type="password"
            bind:value={password}
            required
            minlength={mode === 'login' ? undefined : 8}
            autocomplete={mode === 'login' ? 'current-password' : 'new-password'}
            class="rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </label>
      {/if}

      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {:else if notice}
        <p class="text-sm text-muted-foreground">{notice}</p>
      {/if}

      <Button type="submit" variant="primary" disabled={submitting} class="mt-1">
        {submitting ? 'Please wait…' : title}
      </Button>
    </form>

    {#if mode === 'login'}
      <p class="mt-3 text-center text-sm">
        <button
          type="button"
          onclick={() => switchMode('forgot')}
          class="text-muted-foreground underline-offset-4 hover:underline"
        >
          Forgot your password?
        </button>
      </p>
    {/if}

    {#if isRecovery}
      <p class="mt-4 text-center text-sm text-muted-foreground">
        <button
          type="button"
          onclick={() => switchMode('login')}
          class="font-medium text-foreground underline-offset-4 hover:underline"
        >
          Back to sign in
        </button>
      </p>
    {:else}
      <p class="mt-4 text-center text-sm text-muted-foreground">
        {mode === 'login' ? 'No account?' : 'Already have an account?'}
        <button
          type="button"
          onclick={() => switchMode(mode === 'login' ? 'register' : 'login')}
          class="font-medium text-foreground underline-offset-4 hover:underline"
        >
          {mode === 'login' ? 'Create one' : 'Sign in'}
        </button>
      </p>
    {/if}
</Dialog>
