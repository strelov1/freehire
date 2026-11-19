<script lang="ts">
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
  description="Search tech jobs by meaning, not just keywords. Live progress of semantic embeddings across the freehire catalogue."
  {canonical}
/>

<div class="mx-auto w-full max-w-4xl px-4 py-10 sm:py-14">
  <header class="mb-10">
    <SectionLabel text="Semantic search" />
    <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">Semantic search.</h1>
    <p class="mt-4 max-w-2xl text-lg leading-relaxed text-muted-foreground">
      Every technical posting gets a semantic embedding, which is how freehire finds postings that mean the same thing
      in different words — for “similar jobs”, and for anyone who searches by what the work is rather than what it is
      called. This is how much of the catalogue has one right now — live.
    </p>
  </header>

  <EmbeddingProgress initial={data.progress} streamUrl={api.embedProgressStreamUrl()} />
</div>
