<script lang="ts">
  // The dedicated CV-tailoring workspace: full-width, chat on the left, a tabbed artifact panel
  // (CV · Edit · Job description · Verdict) on the right. Two modes:
  //  - bootstrap (no ?cv): create the tailored CV + a seeded agent session, auto-start, and
  //    store the session id on the CV so it can be re-opened.
  //  - resume (?cv=<id>): reuse the existing CV + its stored session — re-attach, NO kickoff.
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { api, ApiError } from '$lib/api';
  import { createSession } from '$lib/assistant/api';
  import AssistantChat from '$lib/assistant/AssistantChat.svelte';
  import ArtifactPanel from '$lib/tailor/ArtifactPanel.svelte';
  import AccountNavRail from '$lib/components/AccountNavRail.svelte';
  import { currentUser } from '$lib/auth.svelte';
  import type { Analysis } from '$lib/generated/contracts';
  import type { Job } from '$lib/types';

  const slug = $derived(page.params.slug ?? '');
  const cvParam = $derived(page.url.searchParams.get('cv'));
  const eligible = $derived(currentUser()?.beta_tester === true);

  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let errorMsg = $state('');
  let sessionId = $state<string | undefined>(undefined);
  let resuming = $state(false);
  let cvId = $state(0);
  let analysis = $state<Analysis | null>(null);
  let job = $state<Job | null>(null);
  let refreshKey = $state(0);

  const kickoff =
    "Let's tailor my CV for this role — review the fit analysis and walk me through the gaps.";
  const sessionLabel = $derived(job ? `${job.title} · ${job.company}` : undefined);

  onMount(async () => {
    if (!eligible) {
      status = 'error';
      errorMsg = 'CV tailoring is in beta and not available on your account yet.';
      return;
    }
    try {
      if (cvParam) {
        // Resume an existing tailored CV. If it already has a bound session, re-attach it with
        // no kickoff. If it has none (a CV created before session binding), mint a fresh
        // tailoring session for it and let the kickoff orient the agent.
        const existing = Number(cvParam);
        const [j, rec, fit] = await Promise.all([
          api.getJob(slug),
          api.getCv(existing),
          api.getMatchAnalysis(slug).catch(() => null),
        ]);
        job = j;
        cvId = existing;
        analysis = fit?.analysis ?? null;
        if (rec.agent_session_id) {
          resuming = true;
          sessionId = rec.agent_session_id;
        } else {
          const s = await api.startTailorSession(existing);
          sessionId = await createSession({
            cli_token: s.cli_token,
            cv_id: existing,
            base_cv_id: s.base_cv_id,
          });
          await api.setCvSession(existing, sessionId).catch(() => {}); // best-effort
        }
      } else {
        // Bootstrap: create the tailored CV + a seeded session, then bind the session to the CV.
        const [j, tailor] = await Promise.all([api.getJob(slug), api.tailorCv(slug)]);
        job = j;
        cvId = tailor.tailor_cv_id;
        analysis = tailor.analysis;
        sessionId = await createSession({
          cli_token: tailor.cli_token,
          cv_id: tailor.tailor_cv_id,
          base_cv_id: tailor.base_cv_id,
        });
        await api.setCvSession(tailor.tailor_cv_id, sessionId).catch(() => {}); // best-effort
      }
      status = 'ready';
    } catch (e) {
      if (e instanceof ApiError && e.status === 402) {
        // Out of AI credits: surface the message plus when the monthly grant renews.
        const resetsAt = typeof e.body?.resets_at === 'string' ? e.body.resets_at : null;
        const renews = resetsAt
          ? ` They renew ${new Date(resetsAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}.`
          : '';
        errorMsg = `${e.message}${renews}`;
      } else {
        errorMsg = e instanceof ApiError ? e.message : 'Could not open the tailoring workspace.';
      }
      status = 'error';
    }
  });
</script>

<svelte:head><title>Tailor CV{job ? ` · ${job.title}` : ''} — freehire</title></svelte:head>

<!-- Full-width workspace loses the account shell nav; the same left-edge icon rail as
     the Agent page brings the account sections back. It stays put across every state. -->
<div class="flex h-[calc(100svh-3.5rem)]">
  <AccountNavRail />
  {#if status === 'loading'}
    <div class="flex min-w-0 flex-1 items-center justify-center text-sm text-muted-foreground">
      {resuming ? 'Re-opening your tailoring session…' : 'Preparing your tailoring session…'}
    </div>
  {:else if status === 'error'}
    <div class="flex min-w-0 flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <p class="max-w-md text-sm text-destructive">{errorMsg}</p>
      <a href={`/match/${slug}`} class="text-sm text-brand hover:underline">Back to the fit analysis</a>
    </div>
  {:else}
    <div class="flex min-w-0 flex-1">
      <AssistantChat
        session={sessionId}
        kickoff={resuming ? undefined : kickoff}
        {sessionLabel}
        showSessionRail={false}
        onTurnComplete={() => (refreshKey += 1)}
      />
      <ArtifactPanel {cvId} job={job!} {analysis} {refreshKey} />
    </div>
  {/if}
</div>
