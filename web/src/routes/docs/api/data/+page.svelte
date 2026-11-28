<script lang="ts">
  // The paid API plans' extra reference: the job reads that carry the employer's own links and the
  // catalogue export, gathered apart from the full /docs/api so a plan holder finds them first.
  import { page } from '$app/state';
  import ScalarReference from '#lib/components/ScalarReference.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { scalarConfigFromUrl, SCALAR_DATA_SPEC_URL } from '#lib/docs/scalarConfig.js';

  let { data } = $props();

  const canonical = $derived(`${page.url.origin}/docs/api/data`);
</script>

<Seo
  title="freehire API — original job links and catalogue export"
  description="The freehire API and Data plan endpoints: job search and single-job reads with each posting's original employer link, and the Data plan's full-catalogue export."
  {canonical}
/>

<ScalarReference scalarHtml={data.scalarHtml} config={() => scalarConfigFromUrl(SCALAR_DATA_SPEC_URL)} />
