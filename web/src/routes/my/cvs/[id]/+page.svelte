<script lang="ts">
  // Legacy entry point: the standalone CV editor moved into the tailoring workspace tab. Resolve
  // this CV's vacancy slug from the tailored list and redirect into /tailor/<slug>?cv=<id>. A CV
  // that isn't tied to a vacancy (or a load failure) falls back to the tailored-CV list.
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { api } from '$lib/api';

  const id = $derived(Number(page.params.id));

  onMount(async () => {
    try {
      const items = await api.listCvs();
      const match = items.find((cv) => cv.id === id);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to both paths; the rule can't see through the appended ?cv= query
      await goto(match ? `${resolve('/tailor/[slug]', { slug: match.job_slug })}?cv=${id}` : resolve('/my/cvs'), { replaceState: true });
    } catch {
      await goto(resolve('/my/cvs'), { replaceState: true });
    }
  });
</script>

<svelte:head><title>Opening CV… — freehire</title></svelte:head>

<p class="text-muted-foreground">Opening your tailoring workspace…</p>
