<script lang="ts">
  // The public "roast my CV" page: an account-free ATS score plus a live market-coverage
  // reading, over POST /api/v1/cv/roast. Lives at the routes root (not under /my/), so it
  // inherits the ordinary public +layout.svelte rather than the signed-in section — no
  // session is read or required anywhere on this page.
  //
  // Deliberately NOT a $lib/resume.svelte / profileStore consumer: those are session-scoped
  // stores built for the signed-in CV surfaces, and this page's whole point is that no
  // account exists yet. The CV bytes live only in this component's own state and are
  // never sent anywhere but the one POST — see RoastCV's own doc on the backend for why
  // nothing here writes.
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { FileUp, LoaderCircle } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB, type RoastResponse } from '$lib/api';
  import { CATEGORY_OPTIONS } from '$lib/facets';
  import { categoryLabel } from '$lib/labels';
  import { signinUrl } from '$lib/signin';
  import { cn, Button } from '$lib/ui';
  import Seo from '$lib/components/Seo.svelte';
  import ATSReportView from '$lib/components/ATSReportView.svelte';
  import { marketLine, roleDisplay } from './roastView';

  const canonical = $derived(`${page.url.origin}/roast`);

  let fileInput = $state<HTMLInputElement>();
  let dragActive = $state(false);
  let status = $state<'idle' | 'busy' | 'error'>('idle');
  let error = $state<string | null>(null);
  let result = $state<RoastResponse | null>(null);
  // Held so "compare against a different role" can re-post the SAME CV under a new
  // category without asking the visitor to upload it again.
  let lastInput = $state<File | string | null>(null);
  let changingRole = $state(false);
  // Bumped whenever the <select>'s displayed value has to be forced back to the role
  // that was actually measured — a plain assignment to the `value` binding below would
  // not do it, because a native <select> keeps whatever option the visitor clicked even
  // when the request behind it never took effect. Wrapping the element in
  // {#key roleChangeResetKey} makes Svelte tear it down and rebuild it against the
  // current `role`, which is the only reliable way to override that native state.
  let roleChangeResetKey = $state(0);
  // Guards against an out-of-order response: a fast second upload can have an older
  // request resolve after a newer one, so only the latest attempt commits.
  let gen = 0;

  function isPdf(file: File): boolean {
    return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
  }

  async function roast(input: File | string) {
    const my = ++gen;
    status = 'busy';
    error = null;
    lastInput = input;
    // A fresh upload supersedes any role change still in flight — see changeRole's own
    // gen guard, which stops that stale request from ever committing its result, but
    // would otherwise leave the picker disabled forever waiting for a response nothing
    // will act on.
    changingRole = false;
    try {
      const next = await api.roastCv(input);
      if (my !== gen) return;
      result = next;
      status = 'idle';
    } catch (err) {
      if (my !== gen) return;
      result = null;
      status = 'error';
      error = err instanceof ApiError ? err.message : 'Could not read this CV. Please try again.';
    }
  }

  function onFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow re-picking the same file after an error
    if (file) void roast(file);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    if (status === 'busy') return;
    const file = e.dataTransfer?.files?.[0];
    if (!file) return;
    if (!isPdf(file)) {
      error = 'Please drop a PDF file.';
      status = 'error';
      return;
    }
    void roast(file);
  }

  // Re-scores the SAME CV against a different role, leaving the current result on screen
  // if the retry fails rather than clearing a working reading over a transient error.
  // Uses the same out-of-order guard as roast(): a superseding upload must win over a
  // role change that was already in flight, never the other way around.
  async function changeRole(category: string) {
    if (!lastInput || changingRole) return;
    const my = ++gen;
    changingRole = true;
    error = null;
    try {
      const next = await api.roastCv(lastInput, category);
      if (my !== gen) return;
      result = next;
      status = 'idle';
    } catch (err) {
      if (my !== gen) return;
      // The visitor still has the reading they had before — keep it on screen, but say
      // so: a silent failure here left the picker showing a role the reading was never
      // scored against, once the visitor had spent the hourly budget roast() also draws
      // on. Surface it through the same error banner the upload path uses rather than a
      // second error surface, and force the <select> back to the role that reading
      // actually describes.
      status = 'error';
      error = err instanceof ApiError ? err.message : 'Could not change role. Please try again.';
      roleChangeResetKey++;
    } finally {
      changingRole = false;
    }
  }

  function onRoleChange(e: Event) {
    const value = (e.currentTarget as HTMLSelectElement).value;
    if (value) void changeRole(value);
  }

  // Deliberately not the shared "sign in to do X" gate helper from $lib/signin.ts: that
  // one opens the LOGIN form and returns the visitor to the page they came from, which is
  // right for an in-place gate on an existing account (Save, Follow, Vote) but wrong here
  // — this page's audience has never heard of freehire, so there is no account to log
  // into, and returning to /roast would show the same anonymous page with the upload
  // gone. Instead this opens the REGISTER form (no `mode`) and lands a fresh account
  // straight on the signed-in continuation the CTA promised.
  function onSignIn() {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin')
    void goto(signinUrl({ returnTo: '/my/profile/cv-readiness' }));
  }

  const role = $derived(result ? roleDisplay(result) : null);
  const market = $derived(result ? marketLine(result) : null);
</script>

<Seo
  title="Free CV Roast — freehire"
  description="Upload your CV for a free, instant ATS-readiness score and see how many open roles your skills actually reach. No account needed."
  {canonical}
/>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-8 px-4 py-12 sm:py-16">
  <header class="flex flex-col items-center gap-3 text-center">
    <h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">Roast my CV</h1>
    <p class="max-w-xl text-balance text-muted-foreground">
      Drop your CV for an instant, no-account ATS score — and see how many open roles your
      skills actually reach.
    </p>
  </header>

  <input
    type="file"
    accept="application/pdf,.pdf"
    class="hidden"
    bind:this={fileInput}
    onchange={onFile}
  />
  <!-- A drop-target enhancement over the button inside, which is fully keyboard-accessible
       on its own. -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    data-testid="roast-dropzone"
    ondragover={(e) => {
      e.preventDefault();
      if (status !== 'busy') dragActive = true;
    }}
    ondragleave={(e) => {
      e.preventDefault();
      dragActive = false;
    }}
    ondrop={onDrop}
    class={cn(
      'flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed px-6 py-10 text-center transition-colors',
      dragActive ? 'border-brand bg-brand/5' : 'border-border',
    )}
  >
    {#if status === 'busy'}
      <LoaderCircle class="size-6 animate-spin text-muted-foreground" aria-hidden="true" />
      <p class="text-sm font-medium">Reading your CV…</p>
    {:else}
      <FileUp class="size-6 text-muted-foreground" aria-hidden="true" />
      <p class="text-sm font-medium">Drop your CV here, or</p>
      <Button variant="primary" onclick={() => fileInput?.click()}>Choose a PDF</Button>
      <p class="text-xs text-muted-foreground">
        PDF with selectable text, up to {RESUME_MAX_MB} MB. Nothing is stored.
      </p>
    {/if}
  </div>

  {#if status === 'error' && error}
    <p class="text-sm text-destructive" role="alert">{error}</p>
  {/if}

  {#if result}
    <div class="flex flex-col gap-6">
      <ATSReportView report={result.report} />

      <!-- Role line: names what the market reading below was measured against, or says
           plainly that it was not scoped to anything — never presented as if it were. -->
      <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card p-4">
        {#if role?.scoped}
          <p data-testid="roast-role" class="text-sm">
            Measured against <strong>{categoryLabel(role.role)}</strong> postings.
          </p>
        {:else}
          <p data-testid="roast-role-unscoped" class="text-sm text-muted-foreground">
            We couldn't tell which role this CV is for — this reading covers the whole
            catalogue.
          </p>
        {/if}
        <label class="flex items-center gap-2 text-xs text-muted-foreground">
          Compare against
          {#key roleChangeResetKey}
            <select
              data-testid="roast-role-change"
              class="rounded-md border border-input bg-background px-2 py-1 text-xs disabled:opacity-60"
              value={role?.scoped ? role.role : ''}
              disabled={changingRole}
              onchange={onRoleChange}
            >
              <option value="" disabled selected={!role?.scoped}>Pick a role…</option>
              {#each CATEGORY_OPTIONS as opt (opt.value)}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          {/key}
        </label>
      </div>

      <!-- Market line: how far the CV's skills reach into this role's open postings, and
           the single highest-yield skill missing from it. Never a zero when the backend
           could not answer — that would read as a real measurement of nothing found. -->
      {#if market}
        <div class="rounded-xl border border-border bg-card p-4">
          <p class="text-sm">
            Your CV's skills reach <strong class="tabular-nums">{market.covered}</strong> of
            <strong class="tabular-nums">{market.total}</strong> open roles ({market.coveragePercent}%).
          </p>
          {#if market.topGap}
            <p class="mt-1 text-sm text-muted-foreground">
              Adding <strong class="text-foreground">{market.topGap.name}</strong> would unlock
              {market.topGap.unlock_percent}% more.
            </p>
          {/if}
        </div>
      {:else}
        <p data-testid="roast-market-unavailable" class="text-xs text-muted-foreground">
          Market reading is unavailable right now — the score above is unaffected.
        </p>
      {/if}

      <!-- The call to action: everything above is free and account-free by design; the
           model-written review and CV tailoring are what the account is for. -->
      <div class="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border p-6 text-center">
        <p class="text-sm font-medium">Want a model-written review and CV tailoring?</p>
        <p class="text-sm text-muted-foreground">
          Sign in to get an AI review of your CV's content and tailor it to a specific job.
        </p>
        <Button variant="primary" onclick={onSignIn}>Sign in</Button>
      </div>
    </div>
  {/if}
</div>
