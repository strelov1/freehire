<script lang="ts">
  // The dedicated CV-tailoring surface: full-width, chat on the left, a tabbed artifact panel
  // (CV / Job description / Verdict) on the right. On load it bootstraps the tailored CV and a
  // seeded agent session, then auto-starts the agent (kickoff). Reuses <AssistantChat>.
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { api, ApiError } from '$lib/api';
  import { createSession } from '$lib/assistant/api';
  import AssistantChat from '$lib/assistant/AssistantChat.svelte';
  import ArtifactPanel from '$lib/tailor/ArtifactPanel.svelte';
  import { currentUser } from '$lib/auth.svelte';
  import type { Analysis } from '$lib/generated/contracts';
  import type { Job } from '$lib/types';

  const slug = $derived(page.params.slug ?? '');
  const eligible = $derived(
    currentUser()?.beta_tester === true || currentUser()?.role === 'moderator',
  );

  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let errorMsg = $state('');
  let sessionId = $state<string | undefined>(undefined);
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
      // The job (for the JD tab + session name) and the tailoring bootstrap in parallel.
      const [j, tailor] = await Promise.all([api.getJob(slug), api.tailorCv(slug)]);
      job = j;
      cvId = tailor.tailor_cv_id;
      analysis = tailor.analysis;
      sessionId = await createSession({
        cli_token: tailor.cli_token,
        cv_id: tailor.tailor_cv_id,
        base_cv_id: tailor.base_cv_id,
      });
      status = 'ready';
    } catch (e) {
      errorMsg = e instanceof ApiError ? e.message : 'Could not start tailoring. Please try again.';
      status = 'error';
    }
  });
</script>

<svelte:head><title>Tailor CV{job ? ` · ${job.title}` : ''} — freehire</title></svelte:head>

{#if status === 'loading'}
  <div class="flex h-[calc(100svh-3.5rem)] items-center justify-center text-sm text-muted-foreground">
    Preparing your tailoring session…
  </div>
{:else if status === 'error'}
  <div class="flex h-[calc(100svh-3.5rem)] flex-col items-center justify-center gap-3 p-6 text-center">
    <p class="max-w-md text-sm text-destructive">{errorMsg}</p>
    <a href={`/match/${slug}`} class="text-sm text-brand hover:underline">Back to the fit analysis</a>
  </div>
{:else}
  <div class="flex h-[calc(100svh-3.5rem)]">
    <AssistantChat
      session={sessionId}
      {kickoff}
      {sessionLabel}
      showSessionRail={false}
      onTurnComplete={() => (refreshKey += 1)}
    />
    <ArtifactPanel {cvId} job={job!} {analysis} {refreshKey} />
  </div>
{/if}
