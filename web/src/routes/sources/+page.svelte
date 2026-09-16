<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import SourceCatalog from '$lib/components/SourceCatalog.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/sources`);
</script>

<Seo
  title="Sources — freehire"
  description="Every source freehire reads: applicant tracking systems, job aggregators and company career pages, with how many jobs each carries and when we last read it."
  {canonical}
/>

<div class="mx-auto w-full max-w-4xl px-4 py-10 sm:py-14">
  <header class="mb-10">
    <!-- tracking-widest, not the arbitrary tracking-[0.2em] the older pages carry: the
         token-coverage check holds a per-file baseline and a new file starts at zero. -->
    <p class="font-mono text-xs uppercase tracking-widest text-muted-foreground">// sources</p>
    <h1 class="mt-4 text-4xl font-semibold tracking-tighter sm:text-5xl">Where the jobs come from.</h1>
    <p class="mt-4 max-w-2xl text-lg leading-relaxed text-muted-foreground">
      Every place we read postings from, what each one currently carries, and when we last
      read it. For aggregators we also show how much of what they carry we had already found
      at the employer's own applicant tracking system.
    </p>
    <p class="mt-3 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Job counts are the de-duplicated figures search itself shows, so a source's number and
      the search it links to agree. Crawl health in more detail lives on
      <a href={resolve('/status')} class="underline underline-offset-4">the status page</a>.
    </p>
  </header>

  <SourceCatalog sources={data.sources} />
</div>
