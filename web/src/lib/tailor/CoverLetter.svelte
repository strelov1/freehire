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

  // Drafting streams, because the chain is three model calls in series and takes minutes; a
  // proxy closes a silent response at sixty seconds, which is what shipped first and reached
  // every candidate as a 504 while the server was still working.
  //
  // Read with fetch rather than EventSource. EventSource can only GET — and drafting spends an
  // allowance and writes storage, so it must not be a GET — and it hides the status code,
  // which is how a 402 first reached this tab disguised as a dropped connection.
  let controller: AbortController | null = null;

  async function draft() {
    // Guarded here as well as by the disabled attribute: a second run would spend a second
    // allowance and race the first one's write.
    if (drafting) return;
    drafting = true;
    error = '';
    stage = 'select';
    controller = new AbortController();

    try {
      const res = await api.openCoverLetterStream(cvId, band);
      if (!res.ok || !res.body) {
        // A refusal answers before the stream opens, so its status and sentence are both
        // readable here: an exhausted allowance says so, rather than looking like a drop.
        const body = await res.json().catch(() => null);
        error = body?.error ?? 'The letter could not be drafted.';
        return;
      }
      await readStream(res.body);
    } catch (e) {
      if ((e as Error).name === 'AbortError') return;
      // The chain runs detached on the server and stores what it finishes, so a dropped
      // connection is not a lost letter. Saying "try again" would spend a second allowance on
      // work that may already be done.
      error = 'The connection dropped while writing. Your letter may still have been saved — reopen this tab to check.';
    } finally {
      drafting = false;
      stage = '';
      controller = null;
    }
  }

  // Parses the SSE framing by hand: named events separated by blank lines. A malformed frame
  // is skipped rather than thrown, because an uncaught parse here would leave the button
  // spinning forever on a stream that has already ended.
  async function readStream(body: ReadableStream<Uint8Array>) {
    // Decoded by hand rather than through TextDecoderStream: the DOM types disagree about
    // whether that pair accepts Uint8Array or BufferSource, and a decoder with { stream: true }
    // does the same job without the cast.
    const reader = body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    for (;;) {
      // The await-in-loop the linter warns about is the point: the next chunk does not exist
      // until this one is consumed, so there is nothing to parallelise.
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split('\n\n');
      buffer = frames.pop() ?? '';
      for (const frame of frames) handleFrame(frame);
    }
  }

  function handleFrame(frame: string) {
    const name = /^event: (.+)$/m.exec(frame)?.[1];
    const raw = /^data: (.+)$/m.exec(frame)?.[1];
    if (!name || !raw) return; // a keepalive comment, or a frame we do not know
    let data: unknown;
    try {
      data = JSON.parse(raw);
    } catch {
      return;
    }
    if (name === 'stage') {
      const d = data as { stage: string; done: boolean };
      if (!d.done) stage = d.stage;
    } else if (name === 'letter') {
      view = data as CoverLetterView;
    } else if (name === 'stream_error') {
      error = (data as { error: string }).error;
    }
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

  // Aborts a draft still streaming when the tab unmounts, so the reader does not outlive the
  // component. The server side is unaffected: the chain runs on a detached context and stores
  // what it finishes, which is what the candidate's allowance already paid for.
  $effect(() => () => controller?.abort());
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
        onclick={() => void draft()}
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
