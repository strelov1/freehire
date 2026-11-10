<script lang="ts">
  import { api, ApiError } from '#lib/api.js';
  import { announce } from '#lib/announce.svelte.js';
  import type { ModeratedUser } from '#lib/types.js';
  import { Button, FormField } from '#lib/ui/index.js';

  // The manual account ban (internal/api/handler/user_moderation.go): look an account up by
  // id, email or username, see whether it is banned, ban it with a reason, lift the ban.
  // A ban kills every session and key at once and keeps the data.
  let query = $state('');
  let user = $state<ModeratedUser | null>(null);
  let reason = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);

  const message = (err: unknown) => (err instanceof ApiError ? err.message : 'Something went wrong. Please try again.');

  async function run(action: () => Promise<ModeratedUser>, done?: string) {
    busy = true;
    error = null;
    try {
      user = await action();
      reason = '';
      if (done) announce(done);
    } catch (err) {
      error = message(err);
      announce(error, 'assertive');
    } finally {
      busy = false;
    }
  }

  function find(event: SubmitEvent) {
    event.preventDefault();
    const q = query.trim();
    if (!q) return;
    user = null;
    void run(() => api.findUserForModeration(q));
  }
</script>

<div class="flex max-w-xl flex-col gap-6">
  <form class="flex items-end gap-2" onsubmit={find}>
    <div class="flex-1">
      <FormField label="Account id, email or username">
        {#snippet children({ id, describedBy })}
          <input
            {id}
            name="q"
            aria-describedby={describedBy}
            bind:value={query}
            class="w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        {/snippet}
      </FormField>
    </div>
    <Button type="submit" variant="primary" disabled={busy || !query.trim()}>Find</Button>
  </form>

  {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}

  {#if user}
    {@const target = user}
    <section class="rounded-lg border border-border p-4">
      <p class="font-medium">{target.email}</p>
      <p class="text-sm text-muted-foreground">
        #{target.id}{target.username ? ` · @${target.username}` : ''} · {target.role}
      </p>
      {#if target.banned}
        <p class="mt-3 text-sm"><strong>Banned</strong> — {target.ban_reason}</p>
        <Button class="mt-3" disabled={busy} onclick={() => run(() => api.unbanUser(target.id), 'Ban lifted')}>
          Unban
        </Button>
      {:else if target.role === 'admin'}
        <p class="mt-3 text-sm text-muted-foreground">An admin cannot be banned.</p>
      {:else}
        <form
          class="mt-3 flex flex-col gap-3"
          onsubmit={(e) => {
            e.preventDefault();
            void run(() => api.banUser(target.id, reason.trim()), 'Account banned');
          }}
        >
          <FormField label="Reason (required, moderators only)">
            {#snippet children({ id, describedBy })}
              <textarea
                {id}
                name="reason"
                aria-describedby={describedBy}
                rows="3"
                maxlength={500}
                bind:value={reason}
                class="w-full resize-y rounded-lg border border-input bg-transparent px-3 py-2 text-sm focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              ></textarea>
            {/snippet}
          </FormField>
          <div>
            <Button type="submit" variant="destructive" disabled={busy || !reason.trim()}>Ban account</Button>
          </div>
        </form>
      {/if}
    </section>
  {/if}
</div>
