<script lang="ts">
  import { resumeStore } from '#lib/resume.svelte.js';
  import CandidateContactsEditor from '#lib/components/CandidateContactsEditor.svelte';
  import CompletenessBadge from '#lib/components/profile/CompletenessBadge.svelte';

  const resumeMeta = $derived(resumeStore.meta);
</script>

<div class="mb-4">
  <CompletenessBadge section="contacts" />
</div>

<CandidateContactsEditor
  contacts={resumeMeta?.contacts ?? null}
  onSaved={() => void resumeStore.refresh()}
/>
