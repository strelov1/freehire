<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ApiError, api } from '$lib/api';
  import { currentUser } from '$lib/auth.svelte';
  import { Button, Input } from '$lib/ui';

  // Account deletion, stated plainly. This is the one action on the site that cannot
  // be undone on either side — there is no soft-delete, no grace period and no
  // restore — so the surface leads with what disappears and gates the button behind
  // the member typing their own address. An accidental click costs everything.
  let open = $state(false);
  let confirmation = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);

  const email = $derived(currentUser()?.email ?? '');
  const matches = $derived(confirmation.trim().toLowerCase() === email.toLowerCase() && email !== '');

  // What the member is about to lose, named concretely: "your data" tells them nothing.
  const erased = [
    'Your CV, its parsed profile and every CV you built or tailored',
    'Your hosted mailbox, connected Gmail and all stored messages',
    'Saved jobs, applications, reminders and match analyses',
    'AI credits, saved searches, filters and API keys',
    'Your anonymous community handle',
  ];

  async function remove() {
    if (!matches || busy) return;
    busy = true;
    error = null;
    try {
      await api.deleteAccount(confirmation.trim());
      // The account is gone and the session cookie with it; re-resolve to signed-out
      // before leaving so no stale user lingers in the layout.
      await invalidateAll();
      await goto(resolve('/'));
    } catch (e) {
      error =
        e instanceof ApiError
          ? e.message
          : 'Could not delete your account. Nothing was deleted — please try again.';
      busy = false;
    }
  }
</script>

<section class="mt-8 rounded-xl border border-destructive/30 bg-destructive/5 p-5">
  <h2 class="text-sm font-semibold text-destructive">Delete account</h2>
  <p class="mt-1.5 text-sm text-muted-foreground">
    Permanently erases your account and everything in it. This cannot be undone, and we
    cannot restore any of it afterwards.
  </p>

  {#if !open}
    <Button
      variant="ghost"
      size="sm"
      class="mt-3 text-destructive hover:bg-destructive/10 hover:text-destructive"
      onclick={() => (open = true)}
    >
      Delete my account
    </Button>
  {:else}
    <ul class="mt-4 flex list-disc flex-col gap-1 pl-5 text-sm text-muted-foreground">
      {#each erased as item (item)}
        <li>{item}</li>
      {/each}
    </ul>
    <p class="mt-3 text-sm text-muted-foreground">
      Discussions you started stay up so other members don't lose their replies, but your
      handle is removed from them — they are shown as written by a deleted member.
    </p>

    <label class="mt-4 block text-sm font-medium" for="delete-account-confirm">
      Type <span class="font-mono">{email}</span> to confirm
    </label>
    <Input
      id="delete-account-confirm"
      class="mt-1.5 max-w-sm"
      autocomplete="off"
      bind:value={confirmation}
      placeholder={email}
      disabled={busy}
    />

    {#if error}
      <p class="mt-3 text-sm text-destructive">{error}</p>
    {/if}

    <div class="mt-4 flex items-center gap-2">
      <Button variant="destructive" size="sm" disabled={!matches || busy} onclick={remove}>
        {busy ? 'Deleting…' : 'Delete account permanently'}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        disabled={busy}
        onclick={() => {
          open = false;
          confirmation = '';
          error = null;
        }}
      >
        Cancel
      </Button>
    </div>
  {/if}
</section>
