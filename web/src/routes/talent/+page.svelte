<script lang="ts">
  import Seo from '$lib/components/Seo.svelte';
  import TalentView from '$lib/components/TalentView.svelte';
  import type { PageData } from './$types';

  // The route is a thin shell, like /jobs and /companies: the SEO block and the view.
  // Everything else — the filter store, the modal, the pager — lives in TalentView, so
  // the catalogue can be mounted elsewhere without dragging the route with it.

  let { data }: { data: PageData } = $props();
</script>

<Seo
  title="Talent Network — freehire"
  description="Anonymous profiles of candidates open to being approached. Filter by discipline, skills, seniority, timezone and experience."
/>

<TalentView initial={data.page} currentPage={data.currentPage} />
