<script lang="ts">
  import MentorProfileEditor from '$lib/components/MentorProfileEditor.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  // Seeded from the layout's read and held locally so the editor can write back without a
  // round trip through `load`. Creating a profile here is also what makes the tab strip
  // appear — the layout re-reads on the next navigation.
  let profile = $state(data.profile);
</script>

<MentorProfileEditor bind:profile />
