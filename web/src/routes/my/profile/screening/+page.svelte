<script lang="ts">
  import { api } from '$lib/api';
  import ScreeningAnswersForm from '$lib/components/ScreeningAnswersForm.svelte';
  import type { Answers } from '$lib/generated/contracts';

  // Screening answers: a separate concern from the role/skills profile (facts the
  // candidate states directly, not a targeting profile), and its own section rather than
  // a block of Settings — the six fields plus the assistant/autofill wiring behind them
  // earn their own place in the nav. The form re-seeds its own fields from `answers` on
  // reload (dirty-guarded, same pattern as CandidateContactsEditor), since there is no
  // single identity field here to key a remount on.
  let answers = $state<Answers | null>(null);

  // Best-effort: any failure leaves the section on its empty fields rather than erroring.
  async function load() {
    try {
      answers = await api.getScreeningAnswers();
    } catch {
      answers = null;
    }
  }

  $effect(() => {
    void load();
  });
</script>

<svelte:head>
  <title>Screening answers — freehire</title>
</svelte:head>

<ScreeningAnswersForm {answers} onSaved={() => void load()} />
