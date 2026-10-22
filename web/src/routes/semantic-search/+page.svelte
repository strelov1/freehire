<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api } from '#lib/api.js';
  import Seo from '#lib/components/Seo.svelte';
  import EmbeddingProgress from '#lib/components/EmbeddingProgress.svelte';
  import { SectionLabel } from '#lib/ui/index.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/semantic-search`);
</script>

<Seo
  title="Semantic search — freehire"
  description="Search IT jobs by meaning, not just keywords. Live progress of semantic embeddings across the freehire catalogue, and how AI agents use semantic search to find better matches."
  {canonical}
/>

<div class="mx-auto w-full max-w-4xl px-4 py-10 sm:py-14">
  <header class="mb-10">
    <SectionLabel text="semantic search" />
    <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">Semantic search.</h1>
    <p class="mt-4 max-w-2xl text-lg leading-relaxed text-muted-foreground">
      Every technical posting gets a semantic embedding, which is how freehire finds postings
      that mean the same thing in different words — for “similar jobs”, and for anyone who
      searches by what the work is rather than what it is called. This is how much of the
      catalogue has one right now — live.
    </p>
  </header>

  <EmbeddingProgress initial={data.progress} streamUrl={api.embedProgressStreamUrl()} />

  <section class="mt-14">
    <h2 class="text-2xl font-semibold tracking-tight">For AI agents</h2>
    <div class="mt-4 max-w-2xl space-y-4 leading-relaxed text-muted-foreground">
      <p>
        An agent rarely knows the job title it is looking for. It knows what its user wants to do
        — “keep payment systems secure”, “build tools for teachers” — and a keyword search
        misses every posting that says the same thing in other words.
      </p>
      <p>
        So agents can ask for semantic search: the query is embedded and ranked by meaning as
        well as by keywords, over the same embeddings counted above. It is available through the
        MCP server, the CLI and the agent API — set <code class="font-mono text-sm">semantic</code>
        to <code class="font-mono text-sm">true</code> on a search, and the response says whether
        the semantic ranking actually ran. For a plain job title, keywords still answer better.
      </p>
      <p>
        <a
          href={resolve('/agents')}
          class="font-medium text-foreground underline underline-offset-4 hover:no-underline"
          >Connect your agent →</a
        >
      </p>
    </div>
  </section>
</div>
