<script lang="ts">
  // The same workspace component as /tailor/<slug>, in BASE mode: one of the candidate's general
  // CVs, with the agent, and no vacancy.
  import { page } from '$app/state';
  import TailorWorkspace from '#lib/tailor/TailorWorkspace.svelte';
</script>

{#key page.params.id}
  <TailorWorkspace baseCvId={page.params.id ?? ''} />
{/key}
