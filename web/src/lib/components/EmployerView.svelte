<script lang="ts">
  import { browser } from '$app/environment';
  import { Building2, Mail, ShieldAlert } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { format, t } from '$lib/i18n/t';
  import { messages } from './EmployerView.messages';
  import type { EmployerCompany } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import States from './States.svelte';
  import EmployerDashboard from './EmployerDashboard.svelte';

  const s = $derived(t(messages, locale()));

  // The load, distinguishing "no account" (company === null) from "still loading"
  // (status === 'loading') from a genuine fetch failure — three different renders.
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let company = $state<EmployerCompany | null>(null);

  async function load() {
    status = 'loading';
    try {
      company = await api.getEmployerCompany();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }
  $effect(() => {
    if (isAuthenticated()) void load();
  });

  // Whether THIS account's code has already been confirmed, so a page reload while
  // status is still 'pending' shows the "awaiting moderator" message instead of
  // re-asking for a code the backend has already consumed (a second confirm attempt
  // on the same code fails — see internal/identity/accounts' code-store semantics).
  // Per-viewer convenience only, matching the account shell's own nav-collapse flag;
  // never the source of truth for status itself.
  function confirmedKey(slug: string) {
    return `hire.employerClaimConfirmed.${slug}`;
  }
  function markConfirmed(slug: string) {
    if (!browser) return;
    try {
      localStorage.setItem(confirmedKey(slug), '1');
    } catch {
      // Private browsing / blocked storage: the worst case is re-showing the code
      // field on the next reload, not a broken flow.
    }
  }
  function wasConfirmed(slug: string): boolean {
    if (!browser) return false;
    try {
      return localStorage.getItem(confirmedKey(slug)) === '1';
    } catch {
      return false;
    }
  }

  // ── Claim form ─────────────────────────────────────────────────────────
  let companyName = $state('');
  let workEmail = $state('');
  let claiming = $state(false);
  let claimError = $state<string | null>(null);
  // Set once Claim succeeds, so the confirm-code step renders without a second fetch —
  // the server's own response already carries a fresh 'pending' account.
  let justClaimed = $state(false);

  async function submitClaim(e: SubmitEvent) {
    e.preventDefault();
    if (claiming || !companyName.trim() || !workEmail.trim()) return;
    claiming = true;
    claimError = null;
    try {
      company = await api.claimCompany(companyName.trim(), workEmail.trim());
      justClaimed = true;
    } catch (err) {
      claimError = err instanceof ApiError ? err.message : s.loadError;
    } finally {
      claiming = false;
    }
  }

  // ── Confirm step ───────────────────────────────────────────────────────
  let code = $state('');
  let confirming = $state(false);
  let confirmError = $state<string | null>(null);

  async function submitConfirm(e: SubmitEvent) {
    e.preventDefault();
    if (confirming || !code.trim() || !company) return;
    confirming = true;
    confirmError = null;
    try {
      company = await api.confirmEmployerClaim(code.trim());
      markConfirmed(company.company_slug);
      code = '';
      justClaimed = false;
    } catch (err) {
      confirmError = err instanceof ApiError ? err.message : s.loadError;
    } finally {
      confirming = false;
    }
  }

  // The phase this view renders — derived once from company/justClaimed rather than
  // branched inline in the markup, so the markup below reads as one switch.
  const phase = $derived.by(() => {
    if (!company) return 'claim' as const;
    if (company.status === 'active') return 'dashboard' as const;
    if (company.status === 'revoked') return 'revoked' as const;
    // status === 'pending'
    return justClaimed || !wasConfirmed(company.company_slug) ? ('confirm' as const) : ('pending' as const);
  });
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">{s.signedOut}</p>
{:else if status === 'loading'}
  <States state="loading" rows={3} />
{:else if status === 'error'}
  <States state="error" message={s.loadError} />
{:else if phase === 'claim'}
  <div class="flex flex-col gap-6">
    <header class="flex flex-col gap-2 border-b border-border pb-6">
      <p class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-brand-strong">
        <Building2 class="size-3.5" /> {s.headTitle}
      </p>
      <h1 class="text-3xl font-semibold tracking-tight">{s.claimTitle}</h1>
      <p class="max-w-2xl text-sm text-muted-foreground">{s.claimIntro}</p>
    </header>

    <form onsubmit={submitClaim} class="flex max-w-sm flex-col gap-4">
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">{s.companyNameLabel}</span>
        <Input bind:value={companyName} placeholder={s.companyNamePlaceholder} class="w-full" />
      </label>
      <label class="flex flex-col gap-1">
        <span class="flex items-center gap-1.5 text-sm font-medium">
          <Mail class="size-3.5 text-muted-foreground" />
          {s.workEmailLabel}
        </span>
        <Input bind:value={workEmail} type="email" placeholder={s.workEmailPlaceholder} class="w-full" />
        <span class="text-xs text-muted-foreground">{s.workEmailHint}</span>
      </label>
      {#if claimError}
        <p class="text-sm text-destructive">{claimError}</p>
      {/if}
      <Button type="submit" disabled={claiming || !companyName.trim() || !workEmail.trim()}>
        {claiming ? s.claiming : s.claimSubmit}
      </Button>
    </form>
  </div>
{:else if phase === 'confirm'}
  <div class="flex max-w-sm flex-col gap-6">
    <header class="flex flex-col gap-2 border-b border-border pb-6">
      <h1 class="text-2xl font-semibold tracking-tight">{s.confirmTitle}</h1>
      <p class="text-sm text-muted-foreground">{format(s.confirmIntro, { email: company?.work_email ?? '' })}</p>
    </header>
    <form onsubmit={submitConfirm} class="flex flex-col gap-4">
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">{s.codeLabel}</span>
        <Input bind:value={code} inputmode="numeric" maxlength={6} placeholder="123456" class="w-full" />
      </label>
      {#if confirmError}
        <p class="text-sm text-destructive">{confirmError}</p>
      {/if}
      <Button type="submit" disabled={confirming || !code.trim()}>
        {confirming ? s.confirming : s.confirmSubmit}
      </Button>
    </form>
  </div>
{:else if phase === 'pending'}
  <div class="flex items-start gap-3 rounded-lg border border-border bg-secondary/40 p-4 text-sm">
    <ShieldAlert class="mt-0.5 size-5 shrink-0 text-muted-foreground" />
    <div>
      <p class="font-medium">{s.pendingTitle}</p>
      <p class="text-muted-foreground">{format(s.pendingBody, { company: company?.company_name ?? '' })}</p>
    </div>
  </div>
{:else if phase === 'revoked'}
  <div class="flex items-start gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm">
    <ShieldAlert class="mt-0.5 size-5 shrink-0 text-destructive" />
    <div>
      <p class="font-medium">{s.revokedTitle}</p>
      <p class="text-muted-foreground">{format(s.revokedBody, { company: company?.company_name ?? '' })}</p>
    </div>
  </div>
{:else if company}
  <EmployerDashboard {company} onProfileSaved={(updated) => (company = updated)} />
{/if}
