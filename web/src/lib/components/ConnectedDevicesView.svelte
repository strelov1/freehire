<script lang="ts">
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { consumeReauthDraft } from '$lib/recentAuth';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import type { OAuthGrant } from '$lib/types';
  import { Button, ConfirmDialog } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import { messages } from './ConnectedDevicesView.messages';
  import ConfirmIdentity from './ConfirmIdentity.svelte';
  import States from './States.svelte';

  // A list and one dialog, mirroring ApiKeysView: each MCP client a member approved
  // via the OAuth consent screen, with a revoke action gated by the same identity
  // confirmation API-key revocation uses.
  const s = $derived(t(messages, locale()));

  const grantsData = new AsyncData<OAuthGrant[]>([]);
  $effect(() => {
    if (isAuthenticated()) void grantsData.run(() => api.listOAuthGrants());
  });
  const status = $derived(grantsData.status);
  const grants = $derived(grantsData.value);

  let revokeTarget = $state<OAuthGrant | null>(null);
  let confirmRevokeOpen = $state(false);
  let revokePassword = $state('');
  let revokeIdentity = $state<ConfirmIdentity | null>(null);

  // Reopen the revocation the member left to confirm via a provider round trip —
  // same shape as ApiKeysView's pendingRevokeId. The grant ID travels rather than
  // the grant itself, since the list is re-fetched on mount and a stored copy
  // could be of a grant already revoked from another tab.
  $effect(() => {
    const pending = consumeReauthDraft('revoke-oauth-grant');
    if (!pending) return;
    pendingRevokeId = pending.grantId;
  });
  let pendingRevokeId = $state<number | null>(null);
  $effect(() => {
    if (pendingRevokeId === null) return;
    const grant = grantsData.value.find((g) => g.id === pendingRevokeId);
    if (!grant) return;
    pendingRevokeId = null;
    requestRevoke(grant);
  });

  function requestRevoke(grant: OAuthGrant): void {
    revokeTarget = grant;
    revokePassword = '';
    confirmRevokeOpen = true;
  }

  async function revoke(): Promise<void> {
    const grant = revokeTarget;
    if (!grant) return;
    try {
      if (!(await revokeIdentity?.prove())) throw new Error(s.errors.confirmFirst);
      await api.revokeOAuthGrant(grant.id);
      grantsData.value = grantsData.value.filter((g) => g.id !== grant.id);
      revokePassword = '';
    } catch (error) {
      throw new Error(revokeIdentity?.handleRefusal(error) ?? s.errors.revokeFailed, {
        cause: error,
      });
    }
  }
</script>

{#if isAuthenticated()}
  <div class="flex flex-col gap-4">
    <div class="flex flex-col gap-1">
      <h2 class="text-lg font-semibold tracking-tight">{s.title}</h2>
      <p class="text-sm text-muted-foreground">{s.intro}</p>
    </div>

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message={s.errors.loadError} />
    {:else if grants.length === 0}
      <States state="empty" message={s.list.empty} />
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each grants as grant (grant.id)}
          <li class="flex items-center justify-between gap-3 px-4 py-3">
            <div class="flex min-w-0 flex-col gap-0.5">
              <span class="truncate text-sm font-medium">{grant.client_name}</span>
              <span class="text-xs text-muted-foreground">
                {s.list.createdPrefix}
                {timeAgo(grant.created_at, locale())} ·
                {grant.last_used_at
                  ? `${s.list.lastUsedPrefix} ${timeAgo(grant.last_used_at, locale())}`
                  : s.list.neverUsed}
              </span>
            </div>
            <Button variant="ghost" size="sm" onclick={() => requestRevoke(grant)}>
              {s.list.revoke}
            </Button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <ConfirmDialog
    bind:open={confirmRevokeOpen}
    title={`${s.revokeDialog.titlePrefix} "${revokeTarget?.client_name ?? ''}"${s.revokeDialog.titleSuffix}`}
    description={s.revokeDialog.description}
    confirmLabel={s.revokeDialog.confirmLabel}
    variant="destructive"
    onConfirm={revoke}
  >
    <ConfirmIdentity
      bind:this={revokeIdentity}
      bind:password={revokePassword}
      returnTo="/my/api-keys"
      prompt={s.revokeDialog.confirmPrompt}
      draft={() => ({ surface: 'revoke-oauth-grant', grantId: revokeTarget?.id ?? 0 })}
      active={confirmRevokeOpen}
    />
  </ConfirmDialog>
{/if}
