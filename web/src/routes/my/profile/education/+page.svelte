<script lang="ts">
  import { resumeStore } from '#lib/resume.svelte.js';
  import CompletenessBadge from '#lib/components/profile/CompletenessBadge.svelte';
  import EducationCard from '#lib/components/profile/EducationCard.svelte';

  const resumeMeta = $derived(resumeStore.meta);
</script>

<div class="mb-4">
  <CompletenessBadge section="education" />
</div>

<EducationCard
  structured={resumeMeta?.structured ?? null}
  contacts={resumeMeta?.contacts ?? null}
  onSaved={() => void resumeStore.refresh()}
/>
