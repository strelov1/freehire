<script lang="ts">
  import { Mail, UserPlus, Users } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { api, ApiError } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { formatDate } from '#lib/utils.js';
  import { messages } from './EmployerTeam.messages';
  import type { EmployerTeam, EmployerTeamMember } from '#lib/types.js';
  import { Badge, Button, ConfirmDialog, Input } from '#lib/ui/index.js';
  import States from './States.svelte';

  // Leaving ends the caller's access to the company; the host returns to the claim view.
  let { onLeft }: { onLeft: () => void } = $props();

  const s = $derived(t(messages, locale()));
  const me = $derived(currentUser()?.id ?? 0);

  const team = new AsyncData<EmployerTeam | null>(null);
  const load = () => team.run(() => api.getEmployerTeam());
  $effect(() => {
    void load();
  });

  let email = $state('');
  let inviting = $state(false);
  let note = $state<string | null>(null);
  let error = $state<string | null>(null);

  async function invite(e: SubmitEvent) {
    e.preventDefault();
    const to = email.trim();
    if (inviting || !to) return;
    inviting = true;
    note = null;
    error = null;
    try {
      await api.inviteEmployerTeamMember(to);
      email = '';
      note = format(s.invited, { email: to });
      announce(note);
      await load();
    } catch (err) {
      error = err instanceof ApiError ? err.message : s.actionError;
      announce(error, 'assertive');
    } finally {
      inviting = false;
    }
  }

  async function withdraw(id: number) {
    error = null;
    try {
      await api.withdrawEmployerTeamInvitation(id);
      await load();
    } catch (err) {
      error = err instanceof ApiError ? err.message : s.actionError;
    }
  }

  // One confirmation for both removing a colleague and leaving: either ends access at once.
  let confirmOpen = $state(false);
  let target = $state<EmployerTeamMember | null>(null);
  function askRemove(m: EmployerTeamMember) {
    target = m;
    confirmOpen = true;
  }
  async function remove() {
    if (!target) return;
    const leaving = target.user_id === me;
    error = null;
    try {
      await api.removeEmployerTeamMember(target.user_id);
      if (leaving) {
        onLeft();
        return;
      }
      await load();
    } catch (err) {
      error = err instanceof ApiError ? err.message : s.actionError;
    }
  }
</script>

<section class="flex max-w-xl flex-col gap-4 border-t border-border pt-6">
  <div class="flex flex-col gap-1">
    <h2 class="flex items-center gap-1.5 text-sm font-semibold">
      <Users class="size-4 text-muted-foreground" />
      {s.title}
    </h2>
    <p class="text-sm text-muted-foreground">{s.intro}</p>
  </div>

  {#if team.status === 'loading' && !team.value}
    <States state="loading" rows={2} />
  {:else if team.status === 'error' || !team.value}
    <States state="error" message={s.loadError} />
  {:else}
    <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each team.value.members as m (m.user_id)}
        <li class="flex items-center gap-3 px-4 py-2.5">
          <span class="flex min-w-0 flex-1 flex-col">
            <span class="truncate text-sm font-medium">
              {m.work_email}
              {#if m.user_id === me}<span class="font-normal text-muted-foreground"> ({s.you})</span>{/if}
            </span>
            <span class="text-xs text-muted-foreground"
              >{format(s.joined, { date: formatDate(m.joined_at, locale()) })}</span
            >
          </span>
          {#if m.holder}
            <Badge variant="outline">{s.holder}</Badge>
          {:else}
            <Button variant="ghost" size="sm" onclick={() => askRemove(m)}>
              {m.user_id === me ? s.leave : s.remove}
            </Button>
          {/if}
        </li>
      {/each}
    </ul>

    {#if team.value.invitations.length}
      <div class="flex flex-col gap-2">
        <h3 class="text-xs font-medium text-muted-foreground">{s.invitations}</h3>
        <ul class="flex flex-col divide-y divide-border rounded-lg border border-dashed border-border">
          {#each team.value.invitations as inv (inv.id)}
            <li class="flex items-center gap-3 px-4 py-2.5">
              <Mail class="size-4 shrink-0 text-muted-foreground" />
              <span class="flex min-w-0 flex-1 flex-col">
                <span class="truncate text-sm">{inv.email}</span>
                <span class="text-xs text-muted-foreground">
                  {format(s.expires, { date: formatDate(inv.expires_at, locale()) })}
                </span>
              </span>
              <Button variant="ghost" size="sm" onclick={() => withdraw(inv.id)}>{s.withdraw}</Button>
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    <form onsubmit={invite} class="flex flex-col gap-1">
      <label for="team-invite" class="flex items-center gap-1.5 text-sm font-medium">
        <UserPlus class="size-3.5 text-muted-foreground" />
        {s.inviteLabel}
      </label>
      <div class="flex gap-2">
        <Input id="team-invite" bind:value={email} type="email" placeholder={s.invitePlaceholder} class="w-full" />
        <Button type="submit" disabled={inviting || !email.trim()}>{inviting ? s.inviting : s.invite}</Button>
      </div>
      <span class="text-xs text-muted-foreground">{s.inviteHint}</span>
    </form>

    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {:else if note}
      <p class="text-sm text-brand-strong">{note}</p>
    {/if}
  {/if}
</section>

<ConfirmDialog
  bind:open={confirmOpen}
  title={target?.user_id === me
    ? s.leaveConfirmTitle
    : format(s.removeConfirmTitle, { email: target?.work_email ?? '' })}
  description={target?.user_id === me ? s.leaveConfirmBody : s.removeConfirmBody}
  confirmLabel={target?.user_id === me ? s.leave : s.remove}
  cancelLabel={s.cancel}
  onConfirm={remove}
/>
