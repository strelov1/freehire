<script lang="ts">
  import { Check, Copy, LoaderCircle, Play } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { countryLabel } from '#lib/facets.js';
  import { SOURCE_TYPE_LABELS } from '#lib/labels.js';
  import { sourceLabel } from '#lib/sourceLabels.js';
  import type { ProviderKind } from '#lib/types.js';

  // The sources the source picker offers: the largest measured ones the landing already
  // loaded for its source strip, so the demo never offers a source with nothing to find.
  const { sources = [] }: { sources?: { source: string; kind: ProviderKind }[] } = $props();

  // The hero's live demo: a real, keyless request to the public search endpoint, built from
  // a keyword and three filters and shown as the request and the answer it got. The read API
  // answers without a key for evaluation, so the demo needs no sign-in — which is the point
  // it makes. Until the visitor runs it, the panel shows a fixed sample, so the server-rendered
  // page (and a visitor without JavaScript) still sees what a response looks like.

  const ORIGIN = 'https://freehire.me';
  const LIMIT = 3;

  // A short, curated pick of markets — the demo shows the filter, it is not a full picker.
  const COUNTRIES = ['', 'us', 'gb', 'de', 'nl', 'pl', 'fr', 'es', 'ca', 'in', 'br'].map((code) => ({
    code,
    name: code ? countryLabel(code) : 'Anywhere',
  }));
  const SOURCE_TYPES = Object.entries(SOURCE_TYPE_LABELS).map(([code, name]) => ({ code, name }));

  let q = $state('golang');
  let country = $state('de');
  let sourceType = $state('');
  let source = $state('');
  let running = $state(false);
  let copied = $state(false);
  let copiedTimer: ReturnType<typeof setTimeout> | undefined;
  // `status` is the one-line verdict for the status row; `message` is the recovery text,
  // shown in the response pane where it has room on any screen.
  let error = $state<{ status: string; message: string } | null>(null);
  let result = $state<{ total: number; ms: number; body: string } | null>(null);

  // The source picker narrows to the chosen type, so the two filters cannot be set to a
  // combination that matches nothing by construction.
  const sourceOptions = $derived(
    sources
      .filter((s) => !sourceType || s.kind === sourceType)
      .map((s) => ({ code: s.source, name: sourceLabel(s.source) })),
  );
  $effect(() => {
    if (source && !sourceOptions.some((o) => o.code === source)) source = '';
  });

  const query = $derived(
    [
      q.trim() && `q=${encodeURIComponent(q.trim())}`,
      country && `countries=${country}`,
      sourceType && `source_type=${sourceType}`,
      source && `source=${encodeURIComponent(source)}`,
      `limit=${LIMIT}`,
    ]
      .filter(Boolean)
      .join('&'),
  );
  const path = $derived(`/api/v1/jobs/search?${query}`);

  const code = $derived(`curl "${ORIGIN}${path}"
# with a plan: -H "Authorization: Bearer $FREEHIRE_API_KEY"`);

  const SAMPLE = `{
  "data": [{
    "title": "Golang Software Engineer",
    "company": "1global",
    "source": "workable",
    "url": "https://apply.workable.com/j/973E5FE25D",
    "countries": ["de", "pt"],
    "cities": ["Berlin", "Lisbon"],
    "skills": ["go", "grpc", "kafka", "kubernetes", "postgresql"],
    "category": "software_engineering",
    "summary": "Builds and maintains scalable backend systems in Go…",
    "posted_at": "2026-11-03T00:00:00Z",
    "closed_at": null
  }, …],
  "meta": { "total": 142, "limit": 3, "offset": 0 }
}`;

  // What the panel prints of each posting: the fields a buyer decides on, not the whole
  // record — a full posting carries its description and requirements and would bury the
  // shape under one wall of text.
  type Posting = {
    title: string;
    company: string;
    source: string;
    url: string;
    countries?: string[];
    cities?: string[];
    skills?: string[];
    posted_at?: string | null;
    closed_at?: string | null;
    enrichment?: { category?: string; summary?: string } | null;
  };
  function trim(job: Posting) {
    return {
      title: job.title,
      company: job.company,
      source: job.source,
      url: job.url,
      countries: job.countries ?? [],
      cities: job.cities ?? [],
      skills: (job.skills ?? []).slice(0, 5),
      category: job.enrichment?.category ?? null,
      summary: job.enrichment?.summary ?? null,
      posted_at: job.posted_at ?? null,
      closed_at: job.closed_at ?? null,
    };
  }

  // Indented JSON with every array of plain values kept on one line, the way the sample
  // reads — one value per line turns three postings into a page of brackets.
  function pretty(value: unknown): string {
    return JSON.stringify(value, null, 2).replace(
      /\[\s*([^[\]{}]*?)\s*\]/g,
      // Only the newlines and indentation JSON.stringify put between elements are folded;
      // a newline inside a string value is always escaped, so string contents stay intact.
      (_, inner: string) => `[${inner.replace(/,\n\s*/g, ', ')}]`,
    );
  }

  async function run() {
    if (running) return;
    running = true;
    error = null;
    result = null;
    const started = performance.now();
    try {
      const res = await fetch(path, { headers: { accept: 'application/json' } });
      if (res.status === 429) {
        error = {
          status: '429 Too Many Requests',
          message: 'The keyless budget for your address is spent for this minute. Try again shortly.',
        };
        return;
      }
      if (!res.ok) {
        error = {
          status: `${res.status} error`,
          message: 'The API could not answer this request. Try again in a moment.',
        };
        return;
      }
      const body: { data: Posting[]; meta: { total: number; limit: number; offset: number } } = await res.json();
      result = {
        total: body.meta.total,
        ms: Math.round(performance.now() - started),
        body: pretty({ data: body.data.map(trim), meta: body.meta }),
      };
    } catch {
      error = {
        status: 'Request failed',
        message: 'The request did not reach the API. Check your connection and try again.',
      };
    } finally {
      running = false;
    }
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      copied = true;
      announce('Copied to the clipboard');
      clearTimeout(copiedTimer);
      copiedTimer = setTimeout(() => (copied = false), 1500);
    } catch {
      // Clipboard refused (insecure context, permissions): the code stays selectable.
    }
  }
</script>

<div class="min-w-0 overflow-hidden rounded-xl border border-border bg-card" id="playground">
  <form
    class="flex flex-col gap-2 border-b border-border bg-secondary/60 p-3"
    onsubmit={(e) => {
      e.preventDefault();
      run();
    }}
  >
    <div class="flex items-end gap-2">
      <label class="flex min-w-0 flex-1 flex-col gap-1 text-xs text-muted-foreground">
        Keyword
        <input
          bind:value={q}
          maxlength="80"
          class="h-9 rounded-md border border-border bg-background px-2.5 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>
      <button
        type="submit"
        disabled={running}
        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-md bg-primary px-3.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-60"
      >
        {#if running}
          <LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        {:else}
          <Play class="size-4" aria-hidden="true" />
        {/if}
        Run request
      </button>
    </div>
    <div class="grid grid-cols-3 gap-2">
      <label class="flex min-w-0 flex-col gap-1 text-xs text-muted-foreground">
        Country
        <select
          bind:value={country}
          class="h-9 w-full min-w-0 rounded-md border border-border bg-background px-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {#each COUNTRIES as c (c.code)}
            <option value={c.code}>{c.name}</option>
          {/each}
        </select>
      </label>
      <label class="flex min-w-0 flex-col gap-1 text-xs text-muted-foreground">
        Source type
        <select
          bind:value={sourceType}
          class="h-9 w-full min-w-0 rounded-md border border-border bg-background px-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <option value="">Any type</option>
          {#each SOURCE_TYPES as t (t.code)}
            <option value={t.code}>{t.name}</option>
          {/each}
        </select>
      </label>
      <label class="flex min-w-0 flex-col gap-1 text-xs text-muted-foreground">
        Source
        <select
          bind:value={source}
          class="h-9 w-full min-w-0 rounded-md border border-border bg-background px-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <option value="">Any source</option>
          {#each sourceOptions as o (o.code)}
            <option value={o.code}>{o.name}</option>
          {/each}
        </select>
      </label>
    </div>
  </form>

  <div class="flex items-center justify-between border-b border-border px-4 py-1">
    <span class="font-mono text-xs text-muted-foreground">GET /api/v1/jobs/search</span>
    <button
      type="button"
      onclick={copy}
      class="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {#if copied}<Check class="size-3.5" aria-hidden="true" /> Copied{:else}<Copy
          class="size-3.5"
          aria-hidden="true"
        /> Copy{/if}
    </button>
  </div>
  <!-- Wrapped rather than scrolled sideways, so every filter stays readable in the request,
       and a fixed height so the card never changes size. -->
  <pre
    class="h-24 overflow-auto whitespace-pre-wrap break-all border-b border-border px-4 py-3 font-mono text-xs leading-relaxed">{code}</pre>

  <!-- One line, whatever it says: the card keeps a fixed height so the hero beside it never shifts. -->
  <div class="truncate px-4 pt-3 font-mono text-xs text-muted-foreground" aria-live="polite">
    {#if running}
      <span>Requesting…</span>
    {:else if error}
      <span class="text-destructive">{error.status}</span>
    {:else if result?.total === 0}
      <span><span class="text-foreground">200 OK</span> · 0 matches</span>
    {:else if result}
      <span
        ><span class="text-foreground">200 OK</span> · {result.total.toLocaleString('en-US')}
        {result.total === 1 ? 'match' : 'matches'} · {result.ms} ms</span
      >
    {:else}
      <span>Sample response — press Run for a live one, no key needed</span>
    {/if}
  </div>
  {#if error || result?.total === 0}
    <!-- The recovery text lives here rather than in the one-line status row, which would clip it
         on a narrow screen. Same fixed height as the response, so the card keeps its size. -->
    <p class="h-80 px-4 pb-4 pt-2 text-sm leading-relaxed {error ? 'text-destructive' : 'text-muted-foreground'}">
      {error ? error.message : 'No open postings match. Try a broader keyword or fewer filters.'}
    </p>
  {:else}
    <pre
      class="h-80 overflow-auto px-4 pb-4 pt-2 font-mono text-xs leading-relaxed text-muted-foreground">{result?.body ??
        (running ? '' : SAMPLE)}</pre>
  {/if}
</div>
