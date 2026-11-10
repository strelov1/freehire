import type { Http } from '../api-core';
import type {
  AgentPermissions,
  Allowance,
  BankedAnswer,
  ListMeta,
  MyAnalysisItem,
  MyJob,
  MyJobCounts,
  PipelineStats,
  ScheduledInterview,
  TimelineEvent,
  UserJob,
  VoteResult,
  WatchedCompany,
  WatchlistImportResult,
} from '../types';
import { jsonBody, query, toSlice } from '../api-core';
import type { MyJobsFilter, Slice } from '../api-types';

export function trackingApi({ call, request, requestData }: Http) {
  // --- Per-user job interactions --------------------------------------------
  //
  // Both require a session (the auth cookie). Callers gate on auth state before
  // invoking — the SPA never sends these for a signed-out visitor.

  /** Call a job-interaction endpoint and return the resulting record. */
  async function jobInteraction(
    slug: string,
    action: 'view' | 'apply' | 'save' | 'stage' | 'track' | 'dismiss',
    method: 'POST' | 'DELETE' = 'POST',
  ): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/jobs/${encodeURIComponent(slug)}/${action}`, { method });
  }

  /** Record that the current user viewed a job; returns their interaction
   *  (including whether they have already applied). */
  function recordJobView(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'view');
  }

  /** Mark a job as applied for the current user. */
  function markJobApplied(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'apply');
  }

  /** Save (bookmark) a job for the current user. Whether a reminder is scheduled
   *  is decided entirely by the account's notification settings — there is no
   *  per-job override. */
  function saveJob(slug: string): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/jobs/${encodeURIComponent(slug)}/save`, { method: 'POST' });
  }

  /** Set a job's application stage and/or notes (partial update — omit a field to
   *  leave it unchanged). Returns the updated interaction. */
  async function trackJob(slug: string, patch: { stage?: string; notes?: string }): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/jobs/${slug}/track`, jsonBody('PATCH', patch));
  }

  /** Set an application's stage and/or notes, addressing it by the row `id` the tracking
   *  listing served rather than by a posting slug.
   *
   *  The board holds row ids, and an application whose posting the catalogue has removed
   *  has no slug to hold — `trackJob` cannot move it. Same body and same rules,
   *  including the partial update. */
  function trackApplication(id: string, patch: { stage?: string; notes?: string }): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/me/applications/${encodeURIComponent(id)}`, jsonBody('PATCH', patch));
  }

  /** Drop an application's pipeline progress, keeping its saved mark — the board's
   *  backward "move to Saved" drag, addressed by the listing's row id. */
  function clearApplicationStage(id: string): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/me/applications/${encodeURIComponent(id)}/stage`, { method: 'DELETE' });
  }

  /** Remove an application from the board entirely, addressed by the listing's row id. */
  function untrackApplication(id: string): Promise<UserJob> {
    return requestData<UserJob>(`/api/v1/me/applications/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  /** Clear a job's saved mark. Idempotent: "already not saved" is success. */
  function unsaveJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'save', 'DELETE');
  }

  /** Start an auto-apply attempt for the current user and this job
   *  (openspec/changes/auto-apply-submit-trigger). Idempotent for a live, undecided
   *  attempt; rejects with an ApiError (402 not PRO, 409 no base CV or already declined,
   *  400 not a Greenhouse posting) whose `message` is the caller-facing reason. */
  function autoApplyJob(slug: string): Promise<{ status: string }> {
    return requestData<{ status: string }>(`/api/v1/jobs/${encodeURIComponent(slug)}/auto-apply`, {
      method: 'POST',
    });
  }

  /** Record the candidate's approve/decline decision on a tailored-but-not-yet-reviewed
   *  auto-apply queue entry (openspec/changes/auto-apply-review-tracking, calling the
   *  endpoint auto-apply-tailored-resume already built). Rejects with an ApiError (404
   *  foreign/missing entry, 409 no tailored CV yet or already reviewed) whose `message` is
   *  the caller-facing reason. */
  function reviewAutoApply(queueId: string, decision: 'approved' | 'declined'): Promise<{ decision: string }> {
    return requestData<{ decision: string }>(
      `/api/v1/me/auto-apply/${encodeURIComponent(queueId)}/review`,
      jsonBody('POST', { decision }),
    );
  }

  /** Re-arm a stopped auto-apply attempt on this job — parked on a question, failed, or
   *  unable to tailor — or refresh the answer preview of one awaiting review. Rejects with an
   *  ApiError (404 no attempt, 409 not retryable) whose `message` says why. */
  function retryAutoApply(slug: string): Promise<{ retried: boolean }> {
    return requestData<{ retried: boolean }>(`/api/v1/jobs/${encodeURIComponent(slug)}/auto-apply/retry`, {
      method: 'POST',
    });
  }

  /** Withdraw the auto-apply attempt on this job. Rejects with an ApiError (404 no attempt,
   *  409 being submitted right now). */
  function cancelAutoApply(slug: string): Promise<{ cancelled: boolean }> {
    return requestData<{ cancelled: boolean }>(`/api/v1/jobs/${encodeURIComponent(slug)}/auto-apply`, {
      method: 'DELETE',
    });
  }

  function getAgentPermissions(): Promise<AgentPermissions> {
    return requestData<AgentPermissions>('/api/v1/me/agent-permissions');
  }

  /** Cookie-only on the server: no credential an agent holds may grant itself this. */
  function setAgentPermissions(p: AgentPermissions): Promise<AgentPermissions> {
    return requestData<AgentPermissions>('/api/v1/me/agent-permissions', jsonBody('PUT', p));
  }

  /** Save the candidate's answer to one screening question into their answer bank, so it
   *  fills the same question on every later application. */
  async function saveBankedAnswer(question: string, answer: string): Promise<void> {
    await call('/api/v1/me/answer-bank', jsonBody('PUT', { question, answer }));
  }

  /** The candidate's whole answer bank, newest first. */
  function listBankedAnswers(): Promise<BankedAnswer[]> {
    return requestData<BankedAnswer[]>('/api/v1/me/answer-bank');
  }

  /** Remove one banked answer by id. */
  async function deleteBankedAnswer(id: number): Promise<void> {
    await call(`/api/v1/me/answer-bank/${id}`, { method: 'DELETE' });
  }

  /** Dismiss (swipe away) a job in the swipe deck. Keeps it out of the deck only;
   *  the job stays visible in the normal list and search. */
  function dismissJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'dismiss');
  }

  /** Clear a job's dismissed mark (swipe-mode undo). Idempotent: "already not
   *  dismissed" is success. */
  function undismissJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'dismiss', 'DELETE');
  }

  /** Cast a thumbs vote on a job (toggle/flip). Returns the job's resulting public
   *  counters and the caller's own vote. Requires a session — gate on auth first. */
  function voteJob(slug: string, dir: 'up' | 'down'): Promise<VoteResult> {
    return requestData<VoteResult>(`/api/v1/jobs/${encodeURIComponent(slug)}/vote`, jsonBody('POST', { vote: dir }));
  }

  /** Clear the caller's thumbs vote on a job. Idempotent (no-op when none). */
  function clearJobVote(slug: string): Promise<VoteResult> {
    return requestData<VoteResult>(`/api/v1/jobs/${slug}/vote`, { method: 'DELETE' });
  }

  /** Cast a thumbs vote on a company (toggle/flip). Requires a session. */
  function voteCompany(slug: string, dir: 'up' | 'down'): Promise<VoteResult> {
    return requestData<VoteResult>(`/api/v1/companies/${slug}/vote`, jsonBody('POST', { vote: dir }));
  }

  /** Clear the caller's thumbs vote on a company. Idempotent (no-op when none). */
  function clearCompanyVote(slug: string): Promise<VoteResult> {
    return requestData<VoteResult>(`/api/v1/companies/${slug}/vote`, { method: 'DELETE' });
  }

  /** A page of the caller's watchlist, newest added first, optionally narrowed to the
   *  names containing `q`. `max` is the most companies a list may hold. */
  async function listWatchlist(
    q: string,
    limit: number,
    offset: number,
  ): Promise<Slice<WatchedCompany> & { max: number }> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (q) params.set('q', q);
    const page = await request<{ data: WatchedCompany[]; meta: ListMeta & { max: number } }>(
      `/api/v1/me/watchlist?${params}`,
    );
    return { ...toSlice(page, offset), max: page.meta.max };
  }

  /** Whether the caller watches this company. */
  async function isWatched(slug: string): Promise<boolean> {
    return (await requestData<{ watched: boolean }>(`/api/v1/me/watchlist/${encodeURIComponent(slug)}`)).watched;
  }

  /** Put a company on the watchlist. Idempotent; a full list is a 400 whose message says so. */
  function watchCompany(slug: string): Promise<WatchedCompany> {
    return requestData<WatchedCompany>(`/api/v1/me/watchlist/${encodeURIComponent(slug)}`, { method: 'PUT' });
  }

  /** Take a company off the watchlist. Idempotent. */
  async function unwatchCompany(slug: string): Promise<void> {
    await call(`/api/v1/me/watchlist/${encodeURIComponent(slug)}`, { method: 'DELETE' });
  }

  /** Add many companies at once from names or slugs, one per entry. */
  function importWatchlist(names: string[]): Promise<WatchlistImportResult> {
    return requestData<WatchlistImportResult>('/api/v1/me/watchlist/import', jsonBody('POST', { names }));
  }

  /** Drop a job's pipeline progress (stage + applied), keeping its saved mark —
   *  the board's backward "move to Saved" drag. */
  function clearJobStage(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'stage', 'DELETE');
  }

  /** Remove a job from the board entirely (it stays in view history). */
  function untrackJob(slug: string): Promise<UserJob> {
    return jobInteraction(slug, 'track', 'DELETE');
  }

  /** The current user's job interactions, newest activity first. Alongside the
   *  page, the response carries the per-tab counts for the my-jobs tab badges. */
  /** `compact` asks for each card's identity and tag row only — no blurb, skills,
   *  collections or ghost signal. The board draws none of them, and on a 500-card board
   *  they were ~40% of the response, serialized again into the SSR payload. */
  async function listMyJobs(
    filter: MyJobsFilter,
    limit: number,
    offset: number,
    opts: { compact?: boolean } = {},
  ): Promise<Slice<MyJob> & { counts: MyJobCounts }> {
    const compact = opts.compact ? '&compact=1' : '';
    const res = await request<{ data: MyJob[]; meta: ListMeta & { counts: MyJobCounts } }>(
      `/api/v1/me/tracking${query(limit, offset)}&filter=${filter}${compact}`,
    );
    return { ...toSlice(res, offset), counts: res.meta.counts };
  }

  /** The current user's application-pipeline snapshot: per-bucket application
   *  counts aggregated server-side, for the Pipeline tab's Sankey and rate cards. */
  async function getMyPipeline(): Promise<PipelineStats> {
    return requestData<PipelineStats>('/api/v1/me/tracking/pipeline');
  }

  /** What happened to the caller's applications between two instants, oldest first —
   *  the application-event ledger behind the Tracking → Calendar view.
   *
   *  Both bounds are RFC3339 instants and the server does not group them into days: which
   *  day an event falls on depends on the reader's clock, so calendarModel does that here.
   *  Bounds are percent-encoded because a bare `+` in an offset decodes as a space. */
  async function myTimeline(from: string, to: string): Promise<TimelineEvent[]> {
    return requestData<TimelineEvent[]>(
      `/api/v1/me/timeline?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    );
  }

  /** The interviews arranged for the caller's applications whose start falls in the
   *  range — the calendar's second layer, beside what myTimeline reports as having
   *  happened. Cancelled meetings come back marked rather than withheld. */
  async function myInterviews(from: string, to: string): Promise<ScheduledInterview[]> {
    return requestData<ScheduledInterview[]>(
      `/api/v1/me/interviews?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    );
  }

  /** The jobs the current user has run the AI match analysis on (newest first), plus where
   *  they stand on today's analysis allowance — powers the Activity → Matches tab. Never
   *  triggers the LLM. */
  async function myAnalyses(): Promise<{ items: MyAnalysisItem[]; allowance: Allowance | null }> {
    const res = await request<{ data: MyAnalysisItem[]; meta: { allowance: Allowance | null } }>(
      '/api/v1/me/tracking/analyses',
    );
    return { items: res.data, allowance: res.meta.allowance };
  }

  return {
    recordJobView,
    markJobApplied,
    saveJob,
    trackJob,
    trackApplication,
    clearApplicationStage,
    untrackApplication,
    unsaveJob,
    autoApplyJob,
    reviewAutoApply,
    retryAutoApply,
    cancelAutoApply,
    getAgentPermissions,
    setAgentPermissions,
    saveBankedAnswer,
    listBankedAnswers,
    deleteBankedAnswer,
    dismissJob,
    undismissJob,
    voteJob,
    clearJobVote,
    voteCompany,
    clearCompanyVote,
    listWatchlist,
    isWatched,
    watchCompany,
    unwatchCompany,
    importWatchlist,
    clearJobStage,
    untrackJob,
    listMyJobs,
    getMyPipeline,
    myTimeline,
    myInterviews,
    myAnalyses,
  };
}
