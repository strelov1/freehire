<script lang="ts">
  // The Data plan's API reference: the job reads that carry the employer's own links and the
  // catalogue export, gathered apart from the full /docs/api so a plan holder finds them first.
  import { page } from '$app/state';
  import ScalarReference from '#lib/components/ScalarReference.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { scalarConfigFromUrl, SCALAR_DATA_SPEC_URL } from '#lib/docs/scalarConfig.js';

  let { data } = $props();

  const canonical = $derived(`${page.url.origin}/docs/api/data`);
</script>

<Seo
  title="freehire Data plan API — original job links and catalogue export"
  description="The freehire Data plan endpoints: job search and single-job reads with each posting's original employer link, and the full-catalogue export, at 6,000 requests a minute."
  {canonical}
/>

<ScalarReference scalarHtml={data.scalarHtml} config={() => scalarConfigFromUrl(SCALAR_DATA_SPEC_URL)} />
