<script lang="ts">
  // "What's the biggest thing in your way?" — single-select on purpose. A multi-select
  // collects half the list from everyone and loses the ranking that made the answer worth
  // asking for.
  //
  // The free-text box appears only for "Something else", because the server accepts a note
  // ONLY alongside that value and rejects any other pairing: a note beside a coded answer
  // could contradict it, and nothing downstream would know which to believe.
  import OptionList from './OptionList.svelte';
  import { JOB_CHALLENGE_OPTIONS, JOB_CHALLENGE_OTHER } from '$lib/surveyOptions';

  interface Props {
    value: string | null;
    note: string;
    onChange: (value: string) => void;
    onNoteChange: (note: string) => void;
  }

  let { value, note, onChange, onNoteChange }: Props = $props();

  const noteVisible = $derived(value === JOB_CHALLENGE_OTHER);
</script>

<h2 class="text-xl font-semibold tracking-tight">What's the hardest part right now?</h2>
<p class="mt-1 text-sm text-muted-foreground">Pick the one that costs you the most. Optional, like everything here.</p>

<div class="mt-5">
  <OptionList
    options={JOB_CHALLENGE_OPTIONS}
    {value}
    onSelect={onChange}
    label="The hardest part of your job search"
  />
</div>

{#if noteVisible}
  <div class="mt-3">
    <label for="challenge-note" class="mb-2 block text-sm font-medium">Tell us in your own words</label>
    <textarea
      id="challenge-note"
      value={note}
      oninput={(e) => onNoteChange(e.currentTarget.value)}
      rows="3"
      maxlength="500"
      placeholder="What's getting in the way?"
      class="w-full rounded-xl border border-input bg-card px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-ring"
    ></textarea>
  </div>
{/if}
