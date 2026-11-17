<script lang="ts">
  import { Check, Copy, LoaderCircle, Play } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { countryLabel } from '#lib/facets.js';
  import { TabStrip, tabStripId } from '#lib/ui/index.js';

  // The hero's live demo: a real, keyless request to the public search endpoint, built from
  // two inputs and shown as the code that makes it and the answer it got. The read API
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
  const CODE_PANEL = 'jobs-api-playground-code';

  type Lang = 'curl' | 'python' | 'javascript';
  const LANGS: { id: Lang; label: string }[] = [
    { id: 'curl', label: 'cURL' },
    { id: 'python', label: 'Python' },
    { id: 'javascript', label: 'JavaScript' },
  ];

  let q = $state('golang');
  let country = $state('de');
  let lang = $state<Lang>('curl');
  let running = $state(false);
  let copied = $state(false);
  let copiedTimer: ReturnType<typeof setTimeout> | undefined;
  let error = $state<string | null>(null);
  let result = $state<{ total: number; ms: number; body: string } | null>(null);

  const query = $derived(
    [q.trim() && `q=${encodeURIComponent(q.trim())}`, country && `countries=${country}`, `limit=${LIMIT}`]
      .filter(Boolean)
      .join('&'),
  );
  const path = $derived(`/api/v1/jobs/search?${query}`);

  const code = $derived.by(() => {
    const url = `${ORIGIN}${path}`;
    switch (lang) {
      case 'python':
        return `import requests

r = requests.get(
    "${url}",
    # headers={"Authorization": f"Bearer {API_KEY}"},  # with a plan
)
for job in r.json()["data"]:
    print(job["title"], "—", job["company"], job["url"])`;
      case 'javascript':
        return `const res = await fetch(
  "${url}",
  // { headers: { Authorization: \`Bearer \${API_KEY}\` } }, // with a plan
);
const { data, meta } = await res.json();
console.log(meta.total, data.map((j) => j.title));`;
      default:
        return `curl "${url}"
# with a plan: -H "Authorization: Bearer $FREEHIRE_API_KEY"`;
    }
  });

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
      if (res.status === 429)
        throw new Error('The keyless budget for your address is spent for this minute — try again shortly.');
      if (!res.ok) throw new Error(`The API answered ${res.status}.`);
      const body: { data: Posting[]; meta: { total: number; limit: number; offset: number } } = await res.json();
      result = {
        total: body.meta.total,
        ms: Math.round(performance.now() - started),
        body: pretty({ data: body.data.map(trim), meta: body.meta }),
      };
    } catch (e) {
      error = e instanceof Error ? e.message : 'The request failed.';
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
    class="flex flex-wrap items-end gap-2 border-b border-border bg-secondary/60 p-3"
    onsubmit={(e) => {
      e.preventDefault();
      run();
    }}
  >
    <label class="flex min-w-0 flex-1 flex-col gap-1 text-xs text-muted-foreground">
      Keyword
      <input
        bind:value={q}
        maxlength="80"
        class="h-9 rounded-md border border-border bg-background px-2.5 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
    </label>
    <label class="flex flex-col gap-1 text-xs text-muted-foreground">
      Country
      <select
        bind:value={country}
        class="h-9 rounded-md border border-border bg-background px-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {#each COUNTRIES as c (c.code)}
          <option value={c.code}>{c.name}</option>
        {/each}
      </select>
    </label>
    <button
      type="submit"
      disabled={running}
      class="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-60"
    >
      {#if running}
        <LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
      {:else}
        <Play class="size-4" aria-hidden="true" />
      {/if}
      Run request
    </button>
  </form>

  <div class="flex items-center justify-between border-b border-border px-2">
    <TabStrip tabs={LANGS} active={lang} onSelect={(id) => (lang = id)} label="Request language" panelId={CODE_PANEL} />
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
  <div id={CODE_PANEL} role="tabpanel" aria-labelledby={tabStripId(CODE_PANEL, lang)}>
    <pre class="overflow-x-auto border-b border-border px-4 py-3 font-mono text-xs leading-relaxed">{code}</pre>
  </div>

  <div class="flex items-center justify-between px-4 pt-3 font-mono text-xs text-muted-foreground" aria-live="polite">
    {#if running}
      <span>Requesting…</span>
    {:else if error}
      <span class="text-destructive">{error}</span>
    {:else if result?.total === 0}
      <span
        ><span class="text-foreground">200 OK</span> · no open postings match — try a broader keyword or another country</span
      >
    {:else if result}
      <span
        ><span class="text-foreground">200 OK</span> · {result.total.toLocaleString('en-US')} matches · {result.ms} ms</span
      >
    {:else}
      <span>Sample response — press Run for a live one, no key needed</span>
    {/if}
  </div>
  <pre
    class="max-h-80 min-h-40 overflow-auto px-4 pb-4 pt-2 font-mono text-xs leading-relaxed text-muted-foreground">{result?.body ??
      (running || error ? '' : SAMPLE)}</pre>
</div>
