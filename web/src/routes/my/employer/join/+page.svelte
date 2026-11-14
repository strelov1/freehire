<script lang="ts">
  import { Building2 } from '@lucide/svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import AccountPageHeading from '#lib/components/AccountPageHeading.svelte';
  import States from '#lib/components/States.svelte';
  import { api, ApiError } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import type { EmployerTeamInvitationPreview } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { messages } from './messages';

  // The page an invitation email links to. The account shell (my/+layout) owns the auth
  // gate, so a visitor signs in first and lands back here with the token.
  const s = $derived(t(messages, locale()));
  const token = $derived(page.url.searchParams.get('token') ?? '');

  let preview = $state<EmployerTeamInvitationPreview | null>(null);
  let status = $state<'loading' | 'ready' | 'dead'>('loading');
  let joining = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    if (!isAuthenticated()) return;
    if (!token) {
      status = 'dead';
      return;
    }
    void api
      .previewEmployerTeamInvitation(token)
      .then((p) => {
        preview = p;
        status = 'ready';
      })
      .catch(() => (status = 'dead'));
  });

  async function join() {
    if (joining) return;
    joining = true;
    error = null;
    try {
      await api.joinEmployerTeam(token);
      await goto(resolve('/my/employer/vacancies/applications'));
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) error = s.alreadyMember;
      else if (err instanceof ApiError && err.status === 404) status = 'dead';
      else error = s.error;
    } finally {
      joining = false;
    }
  }
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<div class="flex max-w-xl flex-col gap-6">
  {#if status === 'loading'}
    <States state="loading" rows={2} />
  {:else if status === 'dead' || !preview}
    <p class="text-sm text-muted-foreground">{token ? s.dead : s.noToken}</p>
  {:else}
    <header class="flex flex-col gap-2 border-b border-border pb-6">
      <p class="flex items-center gap-1.5 text-xs font-semibold text-brand-strong">
        <Building2 class="size-3.5" />
        {preview.company_name}
      </p>
      <AccountPageHeading>{format(s.title, { company: preview.company_name })}</AccountPageHeading>
      <p class="text-sm text-muted-foreground">
        {format(s.body, { company: preview.company_name, email: preview.email })}
      </p>
    </header>
    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {/if}
    <Button class="w-fit" onclick={join} disabled={joining}>{joining ? s.joining : s.join}</Button>
  {/if}
</div>
