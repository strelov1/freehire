<script lang="ts">
  import { ArrowUp, Check, FileText, LoaderCircle, Plus, X } from '@lucide/svelte';
  import { Button, Chip, lockScroll, SectionLabel, unlockScroll } from '#lib/ui/index.js';
  import Composer from '#lib/assistant/Composer.svelte';
  import ThinkingIndicator from '#lib/assistant/ThinkingIndicator.svelte';
  import PlanLimitLink from '#lib/components/PlanLimitLink.svelte';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import { portal } from '#lib/actions/portal.js';
  import { ApiError, api, isPlanRefusal } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { cvContext, planParams } from '#lib/aiFilter.js';
  import { promptSignIn } from '#lib/signin.js';
  import { errorMessage } from '#lib/utils.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import type { CatalogScale, Interpretation, ResumeProfile } from '#lib/types.js';
  import type { FilterStore } from '#lib/filters.js';
  import AiSearchPlan from './AiSearchPlan.svelte';
  import AiSearchResults, { type Overview } from './AiSearchResults.svelte';
  import { messages } from './AiFilterDialog.messages';

  // The AI search: describe a search in words, check the plan the agent built from it,
  // see what that plan finds, and only then write it to the job list. The server does
  // the interpreting (POST /search/interpret) against the real dictionaries; every
  // message is re-interpreted together with the current plan and answers with a
  // complete replacement, never a diff. Nothing touches the job list until Show N jobs.
  //
  // This file is the shell and the conversation. The plan's rendering is AiSearchPlan,
  // the overview's layout AiSearchResults, the pure logic lib/aiFilter.ts.
  let {
    store,
    onclose,
    onApplied,
  }: { store: FilterStore; onclose: () => void; onApplied?: () => void } = $props();

  type Stage = 'describe' | 'plan' | 'results';
  /** One line of the conversation. `plan` carries the interpretation it shows, so an
   *  older plan keeps rendering what it said; `note` is any plain agent reply, with the
   *  one action it may offer. */
  type Message =
    | { kind: 'you'; text: string; file?: string }
    | { kind: 'plan'; plan: Interpretation }
    | { kind: 'found'; found: number; companies: number | null }
    | { kind: 'note'; text: string; action?: 'start-over' | 'plan-limit' | 'sign-in' };

  const s = $derived(t(messages, locale()));
  const loc = $derived(locale() ?? 'en');
  const fmt = (n: number) => n.toLocaleString(loc);

  let stage = $state<Stage>('describe');
  let draft = $state('');
  let chatDraft = $state('');
  let busy = $state(false);
  let conversation = $state<Message[]>([]);
  // API payloads, only ever reassigned.
  let plan = $state.raw<Interpretation | null>(null);
  let overview = $state.raw<Overview | null>(null);
  let catalog: Promise<CatalogScale | null> | null = null;
  let composer = $state<{ focus: () => void }>();

  // The Run / Keep chatting card sits on the newest plan for as long as it IS the
  // newest plan — an agent note after it (Keep chatting, a failed refinement) does not
  // retire it, a newer plan does.
  const latestPlan = $derived(conversation.findLastIndex((m) => m.kind === 'plan'));
  const planTitle = $derived(plan ? titleCase(plan.query || plan.summary) : '');

  function titleCase(text: string) {
    return text.replace(/\s+/g, ' ').trim().replace(/^./, (c) => c.toUpperCase());
  }

  // The CV is read by the deterministic extractor (no model, nothing stored) and handed
  // to the interpreter as words appended to the first description.
  let cvFile = $state<File | null>(null);
  let cv = $state.raw<ResumeProfile | null>(null);
  let cvBusy = $state(false);
  let cvError = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement>();

  async function onCvPicked(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    cvFile = file;
    cv = null;
    cvError = null;
    cvBusy = true;
    try {
      cv = await api.extractResumeProfile(file);
    } catch (err) {
      cvError = errorMessage(err, s.cvUnreadable);
      cvFile = null;
    } finally {
      cvBusy = false;
    }
  }

  function clearCv() {
    cvFile = null;
    cv = null;
    cvError = null;
    if (fileInput) fileInput.value = '';
  }

  function say(text: string, action?: Extract<Message, { kind: 'note' }>['action']) {
    conversation = [...conversation, { kind: 'note', text, action }];
  }

  // The Describe step's one submit path — Enter, the send button and nothing else. With
  // a CV attached an empty description is still a request: find what fits the CV.
  function submitDescription() {
    if (cvBusy) return;
    const text = draft.trim() || (cv ? s.cvFallback : '');
    void submit(text);
  }

  // Bumped by Start over. A turn still in flight when the visitor starts over belongs to
  // the conversation they abandoned: its answer is dropped rather than becoming the plan
  // the next search is merged into.
  let generation = 0;

  async function submit(text: string) {
    if (!text.trim() || busy) return;
    // "First" is "no plan yet", not "still on Describe": a first interpretation that
    // failed leaves the stage on Search plan, and the retry must carry the CV too.
    const first = plan === null;
    conversation = [...conversation, { kind: 'you', text, file: first ? cvFile?.name : undefined }];
    if (stage === 'describe') stage = 'plan';

    const turn = generation;
    busy = true;
    try {
      const next = await api.interpretSearch(first && cv ? text + cvContext(cv) : text, plan ?? undefined);
      if (turn !== generation) return;
      // Nothing resolved: its own answer, never a plan. Running it would show the whole
      // catalogue and read as "everything matches you".
      if (next.empty) {
        say(s.emptyPlan, 'start-over');
        return;
      }
      plan = next;
      conversation = [...conversation, { kind: 'plan', plan: next }];
      // A refinement made from Results re-runs the overview rather than asking again.
      if (stage === 'results') await runOverview(next);
    } catch (e) {
      if (turn !== generation) return;
      if (isPlanRefusal(e)) say(s.planRefused, 'plan-limit');
      else if (e instanceof ApiError && e.status === 429 && !isAuthenticated()) say(s.guestLimit, 'sign-in');
      else say(errorMessage(e, s.interpretFailed));
    } finally {
      // An abandoned turn must not end the working state of the one that replaced it.
      if (turn === generation) busy = false;
    }
  }

  // The figures for one plan. "Jobs searched" is the published snapshot, never a count
  // on this request path, and it is only shown when the snapshot is exact. Companies
  // ride the facets call; a plan the endpoint will not count companies for (an OR query
  // answers 400) still gets its overview, with that one figure marked unavailable.
  async function loadOverview(p: Interpretation): Promise<Overview> {
    catalog ??= api.catalogScale().catch(() => null);
    const params = planParams(p);
    const facets = api
      .facetCounts(params, { facets: ['countries', 'company_slug'] })
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 400) return api.facetCounts(params, { facets: ['countries'] });
        throw e;
      });
    const [snapshot, counts] = await Promise.all([catalog, facets]);
    return {
      catalogue: snapshot?.exact ? snapshot.unique_open_jobs : null,
      found: counts.total,
      companiesTotal: counts.companies_total ?? null,
      countries: counts.facets.countries ?? {},
      topCompanies: counts.top_companies ?? [],
    };
  }

  async function runOverview(p: Interpretation) {
    try {
      overview = await loadOverview(p);
      const { found, companiesTotal } = overview;
      conversation = [...conversation, { kind: 'found', found, companies: companiesTotal }];
    } catch (e) {
      say(errorMessage(e, s.runFailed));
    }
  }

  async function run() {
    if (!plan || busy) return;
    stage = 'results';
    busy = true;
    try {
      await runOverview(plan);
      // Nothing to show: go back to the plan, where the error note and Run search are
      // in reach at every width, rather than leave an empty Results pane.
      if (!overview) stage = 'plan';
    } finally {
      busy = false;
    }
  }

  // The composer leaves clearing to its host (as AssistantChat does): the field clears
  // only for a message that was accepted, and one sent mid-turn stays where it was.
  function sendChat(text: string) {
    if (busy) return;
    chatDraft = '';
    void submit(text);
  }

  function keepChatting() {
    say(s.keepChattingPrompt);
    composer?.focus();
  }

  function startOver() {
    generation++;
    busy = false;
    stage = 'describe';
    conversation = [];
    plan = null;
    overview = null;
    chatDraft = '';
  }

  // Applying goes through the same query-string path a saved search uses, so an
  // AI-built filter and a hand-built one land identically: one URL write, ordinary
  // removable chips, and the existing Save filter control right beside them.
  function apply() {
    if (!plan) return;
    store.apply(planParams(plan).toString());
    onclose();
    // After this dialog, not after its host's own apply: the write above is already
    // live, so a host that defers its edits (the filter modal) has to hear about it —
    // otherwise it would later apply the filters it staged before this ran, silently
    // undoing them. See AiFilterButton for why closing is the honest response.
    onApplied?.();
  }

  function foundText(found: number, companies: number | null) {
    if (found === 0) return s.foundNothing;
    const jobs = format(plural(loc, found, s.jobs), { count: fmt(found) });
    if (companies == null) return format(s.foundJobs, { jobs });
    return format(s.foundJobsAndCompanies, {
      jobs,
      companies: format(plural(loc, companies, s.companies), { count: fmt(companies) }),
    });
  }

  // Mounted only while open, so the lock lives exactly as long as the dialog does.
  $effect(() => {
    lockScroll();
    return unlockScroll;
  });

  const STAGES: Stage[] = ['describe', 'plan', 'results'];
  const step = $derived(STAGES.indexOf(stage));
</script>

<!-- Moved to the body: this dialog is opened from the filter modal, and before that from
     the filter sidebar, which is `sticky` and so opens a stacking context
     unconditionally. Inside one, `z-50` is scoped to that layer — so the overlay painted
     UNDER the job cards, which come later in the document. No z-index can climb out of a
     stacking context.

     Escape is handled HERE, on the portalled root, rather than on <svelte:window>. Its
     host now has an Escape handler of its own (FilterModalShell), and two window
     listeners both fire for one key — so a single press dismissed this dialog AND the
     modal behind it. Stopping propagation at this element keeps the key from ever
     reaching the window, and the focus trap below guarantees the press lands in here. -->
<div
  class="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-6 lg:p-10"
  onkeydown={(e) => {
    if (e.key !== 'Escape') return;
    e.stopPropagation();
    onclose();
  }}
  {@attach portal()}
>
  <button type="button" aria-label={s.closeDialog} class="absolute inset-0 animate-modal-fade bg-black/50" onclick={onclose}></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-label={s.title}
    class="relative z-10 flex animate-modal-pop max-h-full w-full flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-xl {stage ===
    'describe'
      ? 'max-w-3xl'
      : 'h-full max-w-7xl'}"
    {@attach focusTrap()}
  >
    <div class="flex items-center gap-4 border-b border-border px-5 py-3">
      <div class="flex items-center gap-2">
        <span class="font-semibold">{s.title}</span>
        <Chip variant="brand">{s.beta}</Chip>
      </div>
      <ol class="mx-auto hidden items-center gap-3 text-sm sm:flex">
        {#each STAGES as id, i (id)}
          <li class="flex items-center gap-2" aria-current={i === step ? 'step' : undefined}>
            {#if i < step}
              <span class="grid size-5 place-items-center rounded-full bg-brand text-brand-foreground"><Check class="size-3" /></span>
            {:else}
              <span
                class="grid size-5 place-items-center rounded-full text-xs {i === step
                  ? 'bg-foreground text-background'
                  : 'border border-border text-muted-foreground'}">{i + 1}</span
              >
            {/if}
            <span class={i === step ? 'font-medium' : 'text-muted-foreground'}>{s.steps[id]}</span>
            {#if i < STAGES.length - 1}<span class="h-px w-6 bg-border"></span>{/if}
          </li>
        {/each}
      </ol>
      <button type="button" aria-label={s.close} class="relative ml-auto flex text-muted-foreground after:absolute after:-inset-0.5 hover:text-foreground sm:ml-0" onclick={onclose}>
        <X class="size-5" />
      </button>
    </div>

    {#if stage === 'describe'}
      <div class="overflow-y-auto p-4 sm:p-6">
        <h2 class="text-xl font-semibold">{s.describeHeading}</h2>
        <p class="mt-1 text-sm text-muted-foreground">{s.describeLead}</p>
        <!-- svelte-ignore a11y_autofocus -->
        <textarea
          bind:value={draft}
          rows="5"
          autofocus
          aria-label={s.describeAria}
          placeholder={s.describePlaceholder}
          class="mt-4 w-full resize-none rounded-xl border border-border bg-background p-4 text-base leading-relaxed outline-none transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
          onkeydown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              submitDescription();
            }
          }}
        ></textarea>
        <div class="mt-4 flex items-end justify-between gap-4">
          <input bind:this={fileInput} type="file" accept=".pdf,application/pdf" class="hidden" onchange={onCvPicked} />
          {#if cvFile}
            <div class="flex min-w-0 items-center gap-3 rounded-lg border border-border px-3 py-2 text-sm">
              <span class="grid size-8 shrink-0 place-items-center rounded-md bg-brand-muted text-brand-strong">
                {#if cvBusy}<LoaderCircle class="size-4 animate-spin" />{:else}<FileText class="size-4" />{/if}
              </span>
              <span class="min-w-0">
                <span class="block truncate font-medium">{cvFile.name}</span>
                <span class="block text-xs text-muted-foreground">
                  {#if cvBusy}
                    {s.readingCv}
                  {:else if cv}
                    {cv.seniority ? `${cv.seniority} · ` : ''}{format(plural(loc, cv.skills.length, s.skillsFound), {
                      count: String(cv.skills.length),
                    })}
                  {/if}
                </span>
              </span>
              <button type="button" aria-label={s.removeCv} class="relative text-muted-foreground after:absolute after:-inset-1 hover:text-foreground" onclick={clearCv}>
                <X class="size-4" />
              </button>
            </div>
          {:else}
            <button
              type="button"
              onclick={() => fileInput?.click()}
              class="flex items-center gap-3 rounded-lg border border-dashed border-border px-3 py-2 text-left hover:bg-muted"
            >
              <span class="grid size-8 shrink-0 place-items-center rounded-md bg-brand-muted text-brand-strong"><Plus class="size-4" /></span>
              <span class="text-sm">
                <span class="font-medium">{s.addCv}</span> <span class="text-xs text-muted-foreground">{s.pdf}</span><br />
                <span class={cvError ? 'text-destructive-strong' : 'text-brand-strong'}>{cvError ?? s.clickToUpload}</span>
              </span>
            </button>
          {/if}
          <Button
            variant="primary"
            size="icon"
            class="size-10 shrink-0 rounded-full"
            disabled={(!draft.trim() && !cv) || busy || cvBusy}
            onclick={submitDescription}
            aria-label={s.send}
          >
            <ArrowUp class="size-4" />
          </Button>
        </div>
      </div>
      <div class="border-t border-border bg-muted/40 px-4 py-4 sm:px-6">
        <p class="mb-2 text-sm text-muted-foreground">{s.trySearch}</p>
        <div class="flex flex-wrap gap-2">
          {#each s.examples as example (example)}
            <Button variant="outline" size="sm" class="rounded-full" onclick={() => submit(example)}>{example}</Button>
          {/each}
        </div>
      </div>
    {:else}
      {#snippet chat(compact: boolean)}
        <!-- Follows the conversation down: a reply or the thinking line lands below the
             fold of a long chat, and an answer nobody scrolls to reads as no answer. -->
        <div
          class="flex min-h-0 flex-1 flex-col gap-5 overflow-auto p-4"
          {@attach (el) => {
            void conversation.length;
            void busy;
            el.scrollTop = el.scrollHeight;
          }}
        >
          {#each conversation as m, i (i)}
            {#if m.kind === 'you'}
              <article class="max-w-4/5 self-end rounded-2xl rounded-br-md bg-secondary px-4 py-2.5 text-sm text-secondary-foreground">
                {#if m.file}
                  <span class="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground"><FileText class="size-3.5" /> {m.file}</span>
                {/if}
                {m.text}
              </article>
            {:else if m.kind === 'plan'}
              <AiSearchPlan
                plan={m.plan}
                actionable={stage === 'plan' && i === latestPlan}
                {compact}
                onRun={run}
                onKeepChatting={keepChatting}
              />
            {:else if m.kind === 'found'}
              <p class="text-sm leading-relaxed">{foundText(m.found, m.companies)}</p>
            {:else}
              <div class="space-y-3 text-sm leading-relaxed">
                <p>{m.text}</p>
                {#if m.action === 'start-over'}
                  <Button variant="outline" onclick={startOver}>{s.startOver}</Button>
                {:else if m.action === 'plan-limit'}
                  <PlanLimitLink />
                {:else if m.action === 'sign-in'}
                  <Button variant="primary" onclick={promptSignIn}>{s.signIn}</Button>
                {/if}
              </div>
            {/if}
          {/each}
          {#if busy}<ThinkingIndicator />{/if}
        </div>
        <Composer
            attachable={false}
          bind:this={composer}
          bind:draft={chatDraft}
          queue={[]}
          turnActive={busy}
          disabled={false}
          onSubmit={sendChat}
          onRemoveQueued={() => {}}
        />
      {/snippet}

      {#if stage === 'plan'}
        <div class="mx-auto flex min-h-0 w-full max-w-3xl flex-1 flex-col">
          {#if planTitle}
            <header class="px-6 pt-5 text-center">
              <SectionLabel text={s.searchLabel} />
              <h2 class="mt-1 text-lg font-semibold">{planTitle}</h2>
            </header>
          {/if}
          {@render chat(false)}
        </div>
      {:else if plan}
        <AiSearchResults {plan} {overview} stale={busy} {chat} onApply={apply} />
      {/if}
    {/if}
  </div>
</div>
