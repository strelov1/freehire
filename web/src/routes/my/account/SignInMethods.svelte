<script lang="ts">
  import { replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import ConfirmIdentity from '#lib/components/ConfirmIdentity.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { loadOAuthProviders, PROVIDER_LABELS } from '#lib/oauthProviders.js';
  import { beginProviderLink } from '#lib/providerLink.js';
  import { consumeReauthDraft } from '#lib/recentAuth.js';
  import { Badge, Button, Dialog } from '#lib/ui/index.js';
  import { messages } from './messages';

  // The providers this account can sign in with, and a way to connect one more while signed
  // in. Connecting attaches the provider to THIS account whatever email the provider holds
  // (the server's "link" purpose); before it existed, the only way in was signing in with
  // the provider and hoping its email matched, and a different email made a second account.
  //
  // Connecting adds a way into the account that outlives this session, so it is gated like
  // the other security changes: ConfirmIdentity first, then the trip to the provider. The
  // trip ends on /my/reauth, which sends the outcome back here as `?linked=` or
  // `?link_error=`.

  const s = $derived(t(messages, locale()));
  // The server refuses to attach a provider to an account whose own email is unproven
  // (mobileauth.Store.LinkIdentity); say so before the trip instead of after it.
  const emailVerified = $derived(currentUser()?.email_verified ?? false);

  type Row = { provider: string; linkedAt: string | null };
  const rows = new AsyncData<Row[]>([]);
  function load(): void {
    void rows.run(async () => {
      const [enabled, connected] = await Promise.all([loadOAuthProviders(), api.connectedIdentities()]);
      const linked = new Map(
        connected.identities.filter((i) => i.status === 'active').map((i) => [i.provider, i.linked_at] as const),
      );
      // Apple cannot be linked from the web (its browser flow is re-authentication only), so
      // it is listed only once it is already connected — from the iOS app.
      const names = new Set([...enabled.filter((p) => p !== 'apple'), ...linked.keys()]);
      return [...names]
        .filter((p) => p in PROVIDER_LABELS)
        .sort()
        .map((provider) => ({ provider, linkedAt: linked.get(provider) ?? null }));
    });
  }

  type NoticeKey =
    | 'linked'
    | 'identityInUse'
    | 'providerAlreadyLinked'
    | 'confirmExpired'
    | 'sessionChanged'
    | 'emailUnverified'
    | 'cancelled'
    | 'failed';
  const errorNotice: Record<string, NoticeKey> = {
    identity_in_use: 'identityInUse',
    provider_already_linked: 'providerAlreadyLinked',
    recent_auth_required: 'confirmExpired',
    reauth_session_mismatch: 'sessionChanged',
    email_unverified: 'emailUnverified',
    cancelled: 'cancelled',
  };
  // The KEY and the provider, so a language switch re-renders the sentence.
  let notice = $state<{ key: NoticeKey; provider: string } | null>(null);

  onMount(() => {
    load();
    const params = page.url.searchParams;
    const linked = params.get('linked');
    const failure = params.get('link_error');
    if (!linked && !failure) return;
    notice = linked
      ? { key: 'linked', provider: linked }
      : { key: errorNotice[failure ?? ''] ?? 'failed', provider: '' };
    // Read once: a reload must not announce the same outcome again.
    const qs = [...params.entries()]
      .filter(([key]) => key !== 'linked' && key !== 'link_error')
      .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
      .join('&');
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to the path; the rule can't see through the appended query string
    replaceState(resolve('/my/account') + (qs ? `?${qs}` : ''), page.state);
  });

  const label = (provider: string): string => PROVIDER_LABELS[provider] ?? provider;
  const fill = (text: string, provider: string): string => text.replaceAll('{provider}', label(provider));
  const since = (iso: string): string =>
    new Date(iso).toLocaleDateString(locale(), { year: 'numeric', month: 'long', day: 'numeric' });

  let open = $state(false);
  let target = $state('');
  let password = $state('');
  let busy = $state(false);
  let identity = $state<ConfirmIdentity | null>(null);
  let identityError = $state<string | null>(null);

  // Back from confirming through a provider: reopen on the provider being connected.
  $effect(() => {
    const draft = consumeReauthDraft('link-provider');
    if (!draft) return;
    target = draft.provider;
    open = true;
  });

  $effect(() => {
    if (open) return;
    password = '';
    identityError = null;
  });

  function connect(provider: string): void {
    target = provider;
    notice = null;
    open = true;
  }

  async function proceed(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    if (busy || !target) return;
    identityError = null;
    busy = true;
    try {
      if (!(await identity?.prove())) return;
      // A full-page navigation: on success nothing after this line runs.
      await beginProviderLink(target);
    } catch (err) {
      identityError = identity?.handleRefusal(err) ?? s.signIn.failed;
    } finally {
      busy = false;
    }
  }
</script>

<section class="mb-8 rounded-lg border border-border">
  <div class="p-4">
    <h2 class="text-sm font-semibold">{s.signIn.heading}</h2>
    <p class="text-sm text-muted-foreground">{s.signIn.subheading}</p>
  </div>

  {#if rows.status === 'error'}
    <div class="flex flex-wrap items-center justify-between gap-3 border-t border-border p-4">
      <p class="text-sm text-destructive">{s.signIn.loadError}</p>
      <Button variant="outline" size="sm" onclick={load}>{s.signIn.retry}</Button>
    </div>
  {:else}
    {#each rows.value as row (row.provider)}
      <div class="flex flex-wrap items-center justify-between gap-3 border-t border-border p-4">
        <div class="min-w-0">
          <p class="text-sm font-medium">{label(row.provider)}</p>
          <p class="text-sm text-muted-foreground">
            {row.linkedAt ? `${s.signIn.connectedSince} ${since(row.linkedAt)}` : s.signIn.notConnected}
          </p>
        </div>
        {#if row.linkedAt}
          <Badge variant="secondary">{s.signIn.connected}</Badge>
        {:else if emailVerified}
          <Button variant="outline" size="sm" onclick={() => connect(row.provider)}>{s.signIn.connect}</Button>
        {:else}
          <span class="text-sm text-muted-foreground">{s.signIn.verifyFirst}</span>
        {/if}
      </div>
    {/each}
  {/if}

  {#if notice}
    <p
      role="status"
      class="border-t border-border p-4 text-sm {notice.key === 'linked'
        ? 'text-muted-foreground'
        : 'text-destructive'}"
    >
      {fill(s.signIn[notice.key], notice.provider)}
    </p>
  {/if}
</section>

<Dialog bind:open title={fill(s.signIn.dialogTitle, target)} dismissible={!busy} class="sm:max-w-md">
  <form class="flex flex-col gap-3" onsubmit={proceed}>
    <p class="text-sm text-muted-foreground">{fill(s.signIn.dialogNote, target)}</p>

    <ConfirmIdentity
      bind:this={identity}
      bind:password
      returnTo="/my/account"
      draft={() => ({ surface: 'link-provider', provider: target })}
      prompt={fill(s.signIn.confirmPrompt, target)}
      active={open}
      disabled={busy}
    />

    {#if identityError}
      <p class="text-sm text-destructive">{identityError}</p>
    {/if}

    <div class="mt-1 flex items-center justify-end gap-2">
      <Button variant="ghost" size="sm" disabled={busy} onclick={() => (open = false)}>{s.cancel}</Button>
      <Button type="submit" variant="primary" size="sm" disabled={busy}>
        {busy ? s.signIn.leaving : fill(s.signIn.continue, target)}
      </Button>
    </div>
  </form>
</Dialog>
