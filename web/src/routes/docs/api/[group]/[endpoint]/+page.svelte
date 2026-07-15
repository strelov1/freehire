<script lang="ts">
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import DocsEndpoint from '$lib/components/DocsEndpoint.svelte';

  let { data } = $props();

  const canonical = $derived(`${page.url.origin}${page.url.pathname}`);
</script>

<Seo
  title={`${data.endpoint.method} ${data.endpoint.path} — freehire API`}
  description={data.endpoint.summary}
  {canonical}
/>

<DocsEndpoint
  endpoint={data.endpoint}
  groupTitle={data.groupTitle}
  curlHtml={data.curlHtml}
  responseHtml={data.responseHtml ?? undefined}
  responseLabel={data.responseLabel}
/>
