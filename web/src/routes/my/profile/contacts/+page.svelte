<script lang="ts">
  import { resumeStore } from '#lib/resume.svelte.js';
  import CandidateContactsEditor from '#lib/components/CandidateContactsEditor.svelte';

  const resumeMeta = $derived(resumeStore.meta);
</script>

<CandidateContactsEditor
  contacts={resumeMeta?.contacts ?? null}
  onSaved={() => void resumeStore.refresh()}
/>
