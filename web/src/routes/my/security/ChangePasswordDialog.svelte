<script lang="ts">
  import { ApiError, api } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { Button, Dialog, Input } from '#lib/ui/index.js';
  import { messages } from './messages';

  // Changing a known password, behind a button. Only password accounts see it: an
  // OAuth-only account has none to change and sets one through the forgotten-password flow.

  let { onchanged }: { onchanged?: () => void } = $props();

  const s = $derived(t(messages, locale()));

  let open = $state(false);
  let currentPassword = $state('');
  let password = $state('');
  let confirmation = $state('');
  let saving = $state(false);
  // Stores which message to show, not the resolved string — so a language switch
  // while an error is visible re-renders it in the new language instead of
  // leaving behind a snapshot from whichever locale was active when it occurred.
  type PasswordErrorKey =
    | 'mismatchError'
    | 'wrongCurrentPassword'
    | 'weakPassword'
    | 'staleSession'
    | 'genericError';
  let changeErrorKey = $state<PasswordErrorKey | null>(null);

  // Typed passwords never outlive the dialog. Hangs off `open` because Dialog has three
  // close paths of its own.
  $effect(() => {
    if (open) return;
    currentPassword = password = confirmation = '';
    changeErrorKey = null;
  });

  function keyFor(e: unknown): PasswordErrorKey {
    if (e instanceof ApiError) {
      if (e.status === 401) return 'wrongCurrentPassword';
      if (e.status === 400) return 'weakPassword';
      // 428 is the recent-auth gate. Unlike the other gated surfaces, this one cannot
      // answer it by asking for confirmation: the current password IS the confirmation and
      // the server just accepted it. Reaching here means the proof would not bind to this
      // session — a session minted before per-session ids existed — and the only cure is
      // signing in again. It used to fall through to "Something went wrong", which is both
      // untrue and unactionable.
      if (e.status === 428) return 'staleSession';
    }
    return 'genericError';
  }

  async function changePassword(e: SubmitEvent) {
    e.preventDefault();
    changeErrorKey = null;
    if (password !== confirmation) {
      changeErrorKey = 'mismatchError';
      return;
    }
    saving = true;
    try {
      await api.reauthenticatePassword(currentPassword);
      await api.changePassword(currentPassword, password);
      open = false;
      onchanged?.();
    } catch (err) {
      changeErrorKey = keyFor(err);
    } finally {
      saving = false;
    }
  }
</script>

<Button variant="outline" size="sm" onclick={() => (open = true)}>{s.password.change}</Button>

<Dialog
  bind:open
  title={s.password.dialogTitle}
  description={s.password.subheading}
  dismissible={!saving}
  class="sm:max-w-md"
>
  <form class="flex flex-col gap-3" onsubmit={changePassword}>
    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.password.currentPasswordLabel}</span>
      <Input type="password" bind:value={currentPassword} required autocomplete="current-password" />
    </label>
    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.password.newPasswordLabel}</span>
      <Input
        type="password"
        bind:value={password}
        required
        minlength={8}
        maxlength={72}
        autocomplete="new-password"
      />
    </label>
    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.password.repeatPasswordLabel}</span>
      <Input type="password" bind:value={confirmation} required autocomplete="new-password" />
    </label>

    {#if changeErrorKey}
      <p class="text-sm text-destructive">{s.password[changeErrorKey]}</p>
    {/if}

    <div class="mt-1 flex items-center justify-end gap-2">
      <Button variant="ghost" size="sm" disabled={saving} onclick={() => (open = false)}>{s.cancel}</Button>
      <Button type="submit" variant="primary" size="sm" disabled={saving}>
        {saving ? s.password.saving : s.password.save}
      </Button>
    </div>
  </form>
</Dialog>
