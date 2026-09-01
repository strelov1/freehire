<script lang="ts">
  // The cover letter for the vacancy this CV is being tailored to, and the banked achievements
  // it stands on.
  //
  // This tab carries NO score and no delta, unlike every other tab in this panel. A letter is
  // an artefact rather than a measurement: there is no baseline it could be read against, and
  // putting a number beside it would invite the candidate to optimise prose against a figure
  // that measures nothing.
  //
  // The citation list is the point of the surface, not decoration. It is what makes the claim
  // checkable — every sentence about the candidate's experience traces to something they
  // themselves asserted — so it renders even when it is empty, saying so.
  import { Copy, Check, FileText, Loader2 } from '@lucide/svelte';
  import { api } from '$lib/api';
  import type { CoverLetterView } from '$lib/cv';

  let { cvId }: { cvId: string } = $props();

  // $state.raw: the view is replaced wholesale on every load and never mutated in place, so the
  // deep proxy would cost without buying anything.
  let view = $state.raw<CoverLetterView | null>(null);
  let loading = $state(true);
  let drafting = $state(false);
  let error = $state('');
  let copied = $state(false);
  let band = $state<'short' | 'standard'>('standard');
  // Which step the chain is on — the progress that keeps a minutes-long wait legible instead
  // of looking hung.
  let stage = $state('');

  const stageLabels: Record<string, string> = {
    select: 'Choosing your evidence',
    draft: 'Writing',
    audit: 'Cutting what it cannot support',
  };

  const letter = $derived(view?.letter ?? null);
  // Resolved by the server, claim included. An optional lookup table threaded from the page
  // is what shipped first, and nothing ever passed it — every citation rendered a placeholder
  // while types, tests and linters stayed green.
  const cited = $derived(view?.cited ?? []);

  async function load() {
    loading = true;
    error = '';
    try {
      view = await api.getCoverLetter(cvId);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not read the letter.';
    } finally {
      loading = false;
    }
  }

  // Drafting runs over SSE, not a plain POST. The chain is three model calls in series and
  // takes minutes; a proxy closes a silent response at sixty seconds, which is what shipped
  // first and reached every candidate as a 504 while the server was still working. The
  // stream's first event arrives before any model call, which is what keeps it open.
  function draft() {
    // Guarded here as well as by the disabled attribute: a second run would spend a second
    // allowance and race the first one's write.
    if (drafting) return;
    drafting = true;
    error = '';
    stage = 'select';

    const url = `/api/v1/me/cvs/${encodeURIComponent(cvId)}/cover-letter/stream?band=${band}`;
    const source = new EventSource(url, { withCredentials: true });

    source.addEventListener('stage', (e) => {
      const d = JSON.parse(e.data) as { stage: string; done: boolean };
      if (!d.done) stage = d.stage;
    });
    source.addEventListener('letter', (e) => {
      view = JSON.parse(e.data) as CoverLetterView;
      finish(source);
    });
    source.addEventListener('error', (e) => {
      // Two different failures arrive on this name: our own `error` event, which carries a
      // sentence, and EventSource's transport failure, which carries no data at all. The
      // second one cannot say the letter was lost — the chain runs on a detached context and
      // stores what it finishes — so it says to come back rather than to try again, which
      // would spend a second allowance on work that may already be done.
      const data = (e as MessageEvent).data;
      error = data
        ? ((JSON.parse(data) as { error: string }).error ?? 'The letter could not be drafted.')
        : 'The connection dropped while writing. Your letter may still have been saved — reopen this tab to check.';
      finish(source);
    });
  }

  function finish(source: EventSource) {
    source.close();
    drafting = false;
    stage = '';
  }

  async function copy() {
    if (!letter?.body) return;
    await navigator.clipboard.writeText(letter.body);
    copied = true;
    setTimeout(() => (copied = false), 1500);
  }

  $effect(() => {
    // Re-reads when the workspace switches to another CV. cvId is the only dependency.
    void cvId;
    void load();
  });
</script>

<div class="p-4">
  <header class="mb-3">
    <h3 class="text-sm font-semibold text-foreground">Cover letter</h3>
    <p class="mt-0.5 text-xs leading-snug text-muted-foreground">
      Written from achievements you have confirmed as your own. Nothing here is invented — every
      claim about your experience traces to the evidence listed below it.
    </p>
  </header>

  {#if loading}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else}
    <div class="mb-3 flex flex-wrap items-center gap-2">
      <button
        type="button"
        disabled={drafting}
        onclick={draft}
        class="inline-flex items-center gap-1.5 rounded-md border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50"
      >
        {#if drafting}
          <Loader2 class="size-3.5 animate-spin" aria-hidden="true" />
          {stageLabels[stage] ?? 'Writing'}…
        {:else}
          <FileText class="size-3.5" aria-hidden="true" />
          {letter ? 'Write again' : 'Write the letter'}
        {/if}
      </button>

      <select
        bind:value={band}
        disabled={drafting}
        aria-label="Letter length"
        class="rounded-md border border-border bg-background px-2 py-1 text-xs text-foreground disabled:opacity-50"
      >
        <option value="standard">Standard</option>
        <option value="short">Short</option>
      </select>

      {#if letter?.body}
        <button
          type="button"
          onclick={() => void copy()}
          class="ml-auto inline-flex items-center gap-1.5 rounded-md border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted"
        >
          {#if copied}
            <Check class="size-3.5" aria-hidden="true" />
            Copied
          {:else}
            <Copy class="size-3.5" aria-hidden="true" />
            Copy
          {/if}
        </button>
      {/if}
    </div>

    {#if error}
      <p class="mb-3 text-xs text-destructive">{error}</p>
    {/if}

    {#if view?.stale}
      <!-- Shown, never hidden: a letter written by a retired model is still the letter the
           candidate may already have sent. -->
      <p class="mb-3 rounded-md border border-border bg-muted/30 px-2.5 py-2 text-xs text-muted-foreground">
        This letter was written before the current model or before the vacancy's language was
        re-read. It is still yours to use — write again for a fresh one.
      </p>
    {/if}

    {#if letter?.body}
      <article
        class="whitespace-pre-wrap rounded-xl border border-border bg-muted/30 p-3 text-sm leading-relaxed text-foreground"
      >
        {letter.body}
      </article>

      <section class="mt-3">
        <h4 class="text-xs font-semibold text-foreground">What it stands on</h4>
        {#if cited.length}
          <ul class="mt-1.5 space-y-1">
            <!-- Keyed on the atom id: it is unique by construction, where the claim text is not
                 and a duplicate key takes the whole block down. -->
            {#each cited as atom (atom.id)}
              <li class="text-xs leading-snug text-muted-foreground">
                {#if atom.claim}
                  — {atom.claim}
                {:else}
                  <!-- The owner deleted this evidence after the letter was written. The letter
                       as sent still said what it said, so the citation stays and says so. -->
                  <span class="italic">— an achievement no longer in your bank</span>
                {/if}
              </li>
            {/each}
          </ul>
        {:else}
          <p class="mt-1.5 text-xs leading-snug text-muted-foreground">
            This draft names no specific achievement. Confirm a few in the Experience tab and
            write again — the letter gets sharper the more it has to stand on.
          </p>
        {/if}
      </section>
    {:else if !error}
      <p class="text-sm text-muted-foreground">
        No letter yet for this vacancy. Writing one reads the fit analysis and your experience
        bank, then drafts and edits it down.
      </p>
    {/if}
  {/if}
</div>
