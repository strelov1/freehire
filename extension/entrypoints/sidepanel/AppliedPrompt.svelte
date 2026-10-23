<script lang="ts">
  import { Button, Card } from 'freehire-design-system';
  import type { FreehireJob } from '../../lib/freehire';

  /**
   * Asks whether the application the candidate just left was sent. Asked rather than
   * assumed: leaving a started form is the evidence, not the proof (see
   * `leftAnApplication`). Recording the answer is App.svelte's.
   */
  let {
    job,
    marking = false,
    onConfirm,
    onDismiss,
  }: {
    job: FreehireJob;
    marking?: boolean;
    onConfirm: () => void;
    onDismiss: () => void;
  } = $props();
</script>

<Card class="applied-prompt">
  <p class="question">Did you submit your application?</p>
  <p class="job">{job.title} · {job.company}</p>
  <div class="actions">
    <Button variant="ghost" size="sm" onclick={onDismiss} disabled={marking}>Not yet</Button>
    <Button variant="primary" size="sm" onclick={onConfirm} disabled={marking}>
      {marking ? 'Saving…' : 'Mark as applied'}
    </Button>
  </div>
</Card>

<style>
  /* Same inset as the other cards in the panel. */
  :global(.applied-prompt) {
    padding: 14px;
  }

  .question {
    margin: 0;
    font-size: 13px;
    font-weight: 600;
  }

  .job {
    margin: 4px 0 0;
    font-size: 12px;
    color: var(--muted-foreground);
  }

  .actions {
    margin-top: 10px;
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
