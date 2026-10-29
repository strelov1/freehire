<script lang="ts">
  import ScreeningAnswersForm from '#lib/components/ScreeningAnswersForm.svelte';
  import { screeningStore } from '#lib/screening.svelte.js';

  const answers = $derived(screeningStore.answers);

  // Already warmed by the profile layout (the completeness summary counts this section);
  // ensureLoaded is a no-op when that read has landed, and the fallback when the route is
  // reached directly during a fresh session.
  $effect(() => {
    void screeningStore.ensureLoaded();
  });
</script>

<ScreeningAnswersForm {answers} onSaved={() => void screeningStore.refresh()} />
