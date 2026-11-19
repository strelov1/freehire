<script lang="ts">
  import { Check, Copy, UserSearch } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api, type RecruiterContact } from '#lib/api.js';
  import { isPlanRefusal } from '#lib/api-core.js';
  import { announce } from '#lib/announce.svelte.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { errorMessage } from '#lib/utils.js';
  import { Button } from '#lib/ui/index.js';
  import PlanLimitLink from './PlanLimitLink.svelte';
  import { messages } from './RecruiterContactsBlock.messages';

  // Shown on a job whose detail says a lookup can run (recruiter_contacts_available). A paid
  // reader gets the button, everyone else the plans. Nothing loads until the click: the first
  // view of a job is charged, so it must be the reader's choice.
  let { jobSlug, companyName }: { jobSlug: string; companyName: string } = $props();

  const s = $derived(t(messages, locale()));
  const paid = $derived((currentUser()?.tier ?? 'free') !== 'free');

  let contacts = $state.raw<RecruiterContact[] | null>(null);
  let loading = $state(false);
  let error = $state<string | null>(null);
  let refused = $state(false); // a 402: the server's sentence and the plan link, never a retry
  let copied = $state<string | null>(null);

  async function find() {
    loading = true;
    error = null;
    refused = false;
    try {
      contacts = await api.findRecruiterContacts(jobSlug);
    } catch (e) {
      refused = isPlanRefusal(e);
      error = refused ? errorMessage(e, s.unavailable) : s.unavailable;
      announce(error, 'assertive');
    } finally {
      loading = false;
    }
  }

  async function copy(email: string) {
    try {
      await navigator.clipboard.writeText(email);
    } catch {
      announce(s.copyFailed, 'assertive');
      return;
    }
    copied = email;
    announce(s.copied);
  }
</script>

<section class="flex flex-col gap-3 rounded-lg border border-border px-4 py-3.5">
  <div class="flex items-center gap-4">
    <UserSearch class="size-6 shrink-0 text-muted-foreground" aria-hidden="true" />
    <div class="min-w-0 flex-1">
      <h3 class="text-sm font-semibold">{s.heading}</h3>
      {#if contacts === null}
        <p class="text-xs text-muted-foreground">{format(s.pitch, { company: companyName })}</p>
      {/if}
    </div>
    {#if contacts === null}
      {#if paid}
        <Button variant="outline" size="sm" onclick={find} disabled={loading}>
          {loading ? s.finding : s.find}
        </Button>
      {:else}
        <a href={resolve('/pricing')} class="shrink-0 text-sm font-medium text-brand-strong hover:underline">
          {s.viewPlans}
        </a>
      {/if}
    {/if}
  </div>

  {#if !paid}
    <p class="text-xs text-muted-foreground">{s.paidOnly}</p>
  {/if}

  {#if error}
    <div class="flex flex-wrap items-center gap-2">
      <p role="alert" class="text-sm {refused ? '' : 'text-destructive'}">{error}</p>
      {#if refused}<PlanLimitLink />{/if}
    </div>
  {/if}

  {#if contacts && contacts.length === 0}
    <p class="text-sm text-muted-foreground">{format(s.empty, { company: companyName })}</p>
  {:else if contacts}
    <ul class="flex flex-col divide-y divide-border">
      {#each contacts as c (c.email)}
        <li class="flex items-center gap-3 py-2">
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium">
              {c.name}
              <span
                class="ml-1 rounded-sm px-1.5 py-0.5 text-xs font-normal {c.confidence === 'high'
                  ? 'bg-brand-muted text-brand-strong'
                  : 'bg-muted text-muted-foreground'}">{s[c.confidence]}</span
              >
            </p>
            <p class="truncate text-xs text-muted-foreground">{c.title}</p>
            <a href="mailto:{c.email}" class="text-sm text-brand-strong hover:underline">{c.email}</a>
          </div>
          <button
            type="button"
            onclick={() => copy(c.email)}
            aria-label={format(s.copy, { email: c.email })}
            class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            {#if copied === c.email}<Check class="size-4" />{:else}<Copy class="size-4" />{/if}
          </button>
        </li>
      {/each}
    </ul>
    <p class="text-xs text-muted-foreground">{s.note}</p>
  {/if}
</section>
