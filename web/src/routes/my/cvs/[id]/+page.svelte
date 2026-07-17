<script lang="ts">
  // Legacy entry point: the standalone CV editor moved into the tailoring workspace tab. Resolve
  // this CV's vacancy slug from the tailored list and redirect into /tailor/<slug>?cv=<id>. A CV
  // that isn't tied to a vacancy (or a load failure) falls back to the tailored-CV list.
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { api } from '$lib/api';

  const id = $derived(Number(page.params.id));

  onMount(async () => {
    try {
      const items = await api.listCvs();
      const match = items.find((cv) => cv.id === id);
      await goto(match ? `/tailor/${match.job_slug}?cv=${id}` : '/my/cvs', { replaceState: true });
    } catch {
      await goto('/my/cvs', { replaceState: true });
    }
  });
</script>

<svelte:head><title>Opening CV… — freehire</title></svelte:head>

<p class="text-muted-foreground">Opening your tailoring workspace…</p>
