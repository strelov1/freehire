<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { Button, Card } from 'freehire-design-system';
  import type { AnswerOffer } from '../../lib/answerOffer';

  /**
   * Offers the answers the candidate typed into this form to their answer bank, so
   * the agent fills them on the next application asking the same questions.
   *
   * Every row is a choice, ticked by default: some answers belong to one employer
   * ("agree to arbitrate", "our office 25% of the time"), and banking those silently
   * would have the agent answer them for every other employer too. Saving itself is
   * App.svelte's — this file only collects which offers to keep.
   */
  let {
    offers,
    saving = false,
    onSave,
  }: {
    offers: AnswerOffer[];
    saving?: boolean;
    onSave: (chosen: AnswerOffer[]) => void;
  } = $props();

  let open = $state(false);
  /** Keys the candidate unticked. Tracking the exclusions rather than the picks
   *  keeps an answer that appears while the list is open ticked like the rest. */
  const skipped = new SvelteSet<string>();

  let chosen = $derived(offers.filter((o) => !skipped.has(o.key)));

  function toggle(key: string) {
    if (!skipped.delete(key)) skipped.add(key);
  }
</script>

{#if !open}
  <Button class="w-full" variant="outline" size="sm" onclick={() => (open = true)}>
    Save my answers ({offers.length})
  </Button>
{:else}
  <Card class="save-answers">
    <p class="title">Save to your profile</p>
    <p class="hint">freehire will fill these on the next form that asks. Untick anything only this employer should get.</p>
    <ul>
      {#each offers as offer (offer.key)}
        <li>
          <label>
            <input type="checkbox" checked={!skipped.has(offer.key)} onchange={() => toggle(offer.key)} />
            <span class="text">
              <span class="question">{offer.question}</span>
              <span class="answer">{offer.answer}</span>
            </span>
          </label>
        </li>
      {/each}
    </ul>
    <div class="actions">
      <Button variant="ghost" size="sm" onclick={() => (open = false)} disabled={saving}>Cancel</Button>
      <Button variant="primary" size="sm" onclick={() => onSave(chosen)} disabled={saving || chosen.length === 0}>
        {saving ? 'Saving…' : `Save ${chosen.length}`}
      </Button>
    </div>
  </Card>
{/if}

<style>
  /* Same inset as ApplyPlan, which this card sits under. */
  :global(.save-answers) {
    padding: 14px;
  }

  .title {
    margin: 0;
    font-size: 13px;
    font-weight: 600;
  }

  .hint {
    margin: 4px 0 8px;
    font-size: 12px;
    color: var(--muted-foreground);
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  label {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    cursor: pointer;
  }

  input {
    margin-top: 3px;
    accent-color: var(--brand);
  }

  .text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .question {
    font-size: 13px;
  }

  .answer {
    font-size: 12px;
    color: var(--muted-foreground);
    overflow-wrap: anywhere;
  }

  .actions {
    margin-top: 10px;
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
