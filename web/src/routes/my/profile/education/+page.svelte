<script lang="ts">
  import { resumeStore } from '$lib/resume.svelte';
  import EducationCard from '$lib/components/profile/EducationCard.svelte';

  const resumeMeta = $derived(resumeStore.meta);
</script>

<EducationCard
  structured={resumeMeta?.structured ?? null}
  contacts={resumeMeta?.contacts ?? null}
  onSaved={() => void resumeStore.refresh()}
/>
