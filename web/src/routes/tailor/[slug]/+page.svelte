<script lang="ts">
  import { page } from '$app/state';
  import TailorWorkspace from '#lib/tailor/TailorWorkspace.svelte';
</script>

<TailorWorkspace slug={page.params.slug ?? ''} />
