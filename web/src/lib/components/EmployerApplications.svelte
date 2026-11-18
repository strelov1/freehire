<script lang="ts">
  import { ChevronDown, ChevronRight, FileText, Inbox } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { api, ApiError } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { formatDate } from '#lib/utils.js';
  import { messages } from './EmployerApplications.messages';
  import type { EmployerApplication, EmployerApplicationStage, Job } from '#lib/types.js';
  import { Badge, Button, EmptyState, LoadMore, cn } from '#lib/ui/index.js';
  import { Paginator } from '#lib/paginated.svelte.js';
  import { pillClass, pillCountClass } from './facets/pill';
  import States from './States.svelte';

  const s = $derived(t(messages, locale()));

  // The pipeline in order; the server's EmployerApplicationStage enum names the same six.
  const STAGES: EmployerApplicationStage[] = ['new', 'reviewing', 'interview', 'offer', 'hired', 'rejected'];
  // Mirrors directapply.Stage.Notifies: the moves that email the candidate.
  const NOTIFIES = new Set<EmployerApplicationStage>(['interview', 'offer', 'rejected']);
  const PAGE = 50;

  const selectClass =
    'h-9 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30';

  // Only vacancies that take applications on freehire can have any; the others link out.
  let vacancies = $state<Job[]>([]);
  $effect(() => {
    void api
      .listEmployerVacancies()
      .then((jobs) => (vacancies = jobs.filter((j) => j.direct_apply)))
      .catch(() => (vacancies = []));
  });

  let job = $state('');
  let stage = $state<EmployerApplicationStage | ''>('');
  let counts = $state<Record<string, number>>({});

  // The shared Paginator owns the list, its paging and its load states (and drops a response
  // a newer filter superseded); the fetch reads the live filters at call time, so a filter
  // change just restarts it from the first page. Each page also carries the stage counts
  // under the vacancy filter, applied only while that filter is still the one shown.
  const pager = new Paginator<EmployerApplication>(
    async (limit, offset) => {
      const filter = { job, stage: stage || undefined };
      const page = await api.listEmployerApplications({ ...filter, limit, offset });
      if (filter.job === job && (filter.stage ?? '') === stage) counts = page.stageCounts;
      return page;
    },
    { limit: PAGE, keyOf: (a) => a.id },
  );
  $effect(() => {
    void job;
    void stage;
    pager.status = 'loading';
    void pager.start();
  });

  const allCount = $derived(Object.values(counts).reduce((a, n) => a + n, 0));
  const vacancyTitle = (slug: string) => vacancies.find((v) => v.public_slug === slug)?.title ?? slug;

  // ── One expanded application and its move form ─────────────────────────
  let openId = $state<number | null>(null);
  let target = $state<EmployerApplicationStage>('new');
  let message = $state('');
  let moving = $state(false);
  // The last move's outcome, shown above the list rather than inside the row: a row moved
  // out of the stage filter disappears, and the outcome (above all "the email failed") must
  // not disappear with it. It names the candidate, since another application may be open by
  // the time it arrives.
  let outcome = $state<{ text: string; tone: 'ok' | 'error' } | null>(null);

  function toggle(a: EmployerApplication) {
    if (openId === a.id) {
      openId = null;
      return;
    }
    openId = a.id;
    target = a.stage;
    message = '';
  }

  async function move(a: EmployerApplication) {
    if (moving || target === a.stage) return;
    moving = true;
    outcome = null;
    const sent = { job, stage };
    try {
      const moved = await api.moveEmployerApplication(
        a.id,
        target,
        NOTIFIES.has(target) ? message.trim() || undefined : undefined,
      );
      const next = moved.application;
      const filtersUnchanged = sent.job === job && sent.stage === stage;
      if (filtersUnchanged && stage && next.stage !== stage) {
        // It left the list shown: reload it, so the counts are fresh and the next page's
        // offset still lines up (a hole in the loaded window would skip an applicant).
        void pager.start();
      } else if (filtersUnchanged) {
        counts = {
          ...counts,
          [a.stage]: Math.max(0, (counts[a.stage] ?? 0) - 1),
          [next.stage]: (counts[next.stage] ?? 0) + 1,
        };
        pager.items = pager.items.map((x) => (x.id === a.id ? next : x));
      }
      // Only the form this move came from is reset; another application opened meanwhile keeps
      // what was typed into it.
      if (openId === a.id) message = '';
      const text = !NOTIFIES.has(next.stage) ? s.moved : moved.candidate_notified ? s.movedNotified : s.movedMailFailed;
      outcome = {
        text: format(s.movedOutcome, { email: a.candidate_email, outcome: text }),
        tone: NOTIFIES.has(next.stage) && !moved.candidate_notified ? 'error' : 'ok',
      };
      announce(outcome.text);
    } catch (err) {
      const text = err instanceof ApiError ? err.message : s.moveError;
      outcome = { text: format(s.movedOutcome, { email: a.candidate_email, outcome: text }), tone: 'error' };
      announce(outcome.text, 'assertive');
    } finally {
      moving = false;
    }
  }

  function readable(type: string, answer: string): string {
    if (type === 'boolean') return answer === 'true' ? s.yes : s.no;
    return answer;
  }

  const stageHint = (st: EmployerApplicationStage) =>
    st === 'new' ? s.newOnly : NOTIFIES.has(st) ? s.emailsCandidate : s.tracksOnly;
</script>

<div class="flex flex-col gap-4">
  <p class="max-w-2xl text-sm text-muted-foreground">{s.intro}</p>

  <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
    <div class="flex flex-wrap gap-1.5" role="group" aria-label={s.stageLabel}>
      {#each [['', s.allStages, allCount], ...STAGES.map( (st) => [st, s.stages[st], counts[st] ?? 0] )] as [value, label, n] (value)}
        <button
          type="button"
          aria-pressed={stage === value}
          onclick={() => (stage = value as EmployerApplicationStage | '')}
          class={pillClass(stage === value, false, 'inline-flex items-center px-3 py-1 text-xs')}
        >
          {label}
          <span class={pillCountClass(stage === value)}>{n}</span>
        </button>
      {/each}
    </div>
    {#if vacancies.length > 1}
      <label class="flex items-center gap-2 text-sm">
        <span class="text-muted-foreground">{s.vacancyFilter}</span>
        <select bind:value={job} class={selectClass}>
          <option value="">{s.allVacancies}</option>
          {#each vacancies as v (v.public_slug)}
            <option value={v.public_slug}>{v.title}</option>
          {/each}
        </select>
      </label>
    {/if}
  </div>

  {#if outcome}
    <p role="status" class={outcome.tone === 'error' ? 'text-sm text-destructive' : 'text-sm text-brand-strong'}>
      {outcome.text}
    </p>
  {/if}

  {#if pager.status === 'loading'}
    <States state="loading" rows={3} />
  {:else if pager.status === 'error'}
    <States state="error" message={s.loadError} />
  {:else if pager.items.length === 0}
    <EmptyState title={job || stage ? s.emptyFiltered : s.empty}>
      {#snippet icon()}<Inbox class="size-6" />{/snippet}
    </EmptyState>
  {:else}
    <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each pager.items as a (a.id)}
        <li>
          <button
            type="button"
            class="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-secondary/40"
            aria-expanded={openId === a.id}
            onclick={() => toggle(a)}
          >
            {#if openId === a.id}
              <ChevronDown class="size-4 shrink-0 text-muted-foreground" />
            {:else}
              <ChevronRight class="size-4 shrink-0 text-muted-foreground" />
            {/if}
            <span class="flex min-w-0 flex-1 flex-col">
              <span class="truncate text-sm font-medium">{a.candidate_email}</span>
              <span class="truncate text-xs text-muted-foreground">
                {job ? '' : `${vacancyTitle(a.job_slug)} · `}{format(s.sentOn, {
                  date: formatDate(a.sent_at, locale()),
                })}
              </span>
            </span>
            <Badge variant={a.stage === 'new' ? 'brand' : a.stage === 'rejected' ? 'outline' : 'secondary'}>
              {s.stages[a.stage]}
            </Badge>
          </button>

          {#if openId === a.id}
            <div class="flex flex-col gap-4 border-t border-border px-4 py-4 sm:pl-11">
              {#if a.answers.length}
                <dl class="flex flex-col gap-2 text-sm">
                  {#each a.answers as ans (ans.id)}
                    <div>
                      <dt class="text-xs text-muted-foreground">{ans.label}</dt>
                      <dd class="whitespace-pre-line">{readable(ans.type, ans.answer)}</dd>
                    </div>
                  {/each}
                </dl>
              {:else}
                <p class="text-xs text-muted-foreground">{s.noAnswers}</p>
              {/if}

              {#if a.note}
                <div class="text-sm">
                  <p class="text-xs text-muted-foreground">{s.noteLabel}</p>
                  <blockquote class="whitespace-pre-line border-l-2 border-border pl-3">{a.note}</blockquote>
                </div>
              {/if}

              <div class="flex flex-wrap items-center gap-2 text-sm">
                {#if a.has_cv}
                  <Button
                    variant="outline"
                    size="sm"
                    class="gap-1.5"
                    href={api.employerApplicationCVUrl(a.id)}
                    target="_blank"
                    rel="noopener"
                  >
                    <FileText class="size-4" />
                    {s.openCV}
                  </Button>
                  <span class="text-xs text-muted-foreground">
                    {a.cv_kind === 'tailored' ? s.cvTailored : s.cvUploaded}
                  </span>
                {:else}
                  <span class="text-xs text-muted-foreground">{s.cvEmailOnly}</span>
                {/if}
              </div>

              <form
                class="flex max-w-xl flex-col gap-3 rounded-lg border border-border p-3"
                onsubmit={(e) => {
                  e.preventDefault();
                  void move(a);
                }}
              >
                <label class="flex flex-col gap-1">
                  <span class="text-sm font-medium">{s.stageLabel}</span>
                  <select bind:value={target} class={cn(selectClass, 'w-full sm:w-60')}>
                    {#each STAGES as st (st)}
                      <option value={st}>{s.stages[st]}</option>
                    {/each}
                  </select>
                  {#if target !== a.stage}
                    <span class="text-xs text-muted-foreground">{stageHint(target)}</span>
                  {/if}
                </label>
                {#if target !== a.stage && NOTIFIES.has(target)}
                  <label class="flex flex-col gap-1">
                    <span class="text-sm font-medium">{s.messageLabel}</span>
                    <textarea
                      bind:value={message}
                      rows="3"
                      maxlength={2000}
                      class="w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30"
                    ></textarea>
                    <span class="text-xs text-muted-foreground">{s.messageHint}</span>
                  </label>
                {/if}
                <Button type="submit" size="sm" class="w-fit" disabled={moving || target === a.stage}>
                  {moving ? s.saving : s.save}
                </Button>
              </form>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
    {#if pager.hasMore}
      <LoadMore loading={pager.loadingMore} error={pager.loadMoreError} onclick={() => pager.loadMore()} />
    {/if}
  {/if}
</div>
