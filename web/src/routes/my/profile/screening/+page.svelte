<script lang="ts">
  import { api } from '$lib/api';
  import ScreeningAnswersForm from '$lib/components/ScreeningAnswersForm.svelte';
  import type { Answers } from '$lib/generated/contracts';

  let screeningAnswers = $state<Answers | null>(null);

  // Best-effort — a failure here leaves the section blank on next load rather than
  // erroring the whole page.
  async function loadScreeningAnswers() {
    try {
      screeningAnswers = await api.getScreeningAnswers();
    } catch {
      screeningAnswers = null;
    }
  }

  $effect(() => {
    void loadScreeningAnswers();
  });
</script>

<ScreeningAnswersForm answers={screeningAnswers} onSaved={() => void loadScreeningAnswers()} />
