<script lang="ts">
  import { Button, Card } from 'freehire-design-system';
  import { track } from '../../lib/analytics';
  import { getToken } from '../../lib/auth';
  import { markApplied, type FreehireJob } from '../../lib/freehire';

  /**
   * Asks whether the application the candidate just left was sent. Asked rather than
   * assumed: leaving a started form is the evidence, not the proof (see
   * `leftAnApplication`). A "yes" records the application; either answer ends the prompt
   * (`onDone`), and the outcome is reported as a panel notice.
   */
  let {
    job,
    onNotice,
    onDone,
  }: {
    job: FreehireJob;
    onNotice: (text: string) => void;
    onDone: () => void;
  } = $props();

  let marking = $state(false);

  async function confirm() {
    const token = await getToken();
    if (!token || marking) return;
    marking = true;
    try {
      await markApplied(job.public_slug, token);
      track('mark_applied', { source: 'left_form' });
      onNotice(`✓ Marked as applied — ${job.title} is in your Applied list.`);
      onDone();
    } catch (err) {
      onNotice(`Could not mark as applied: ${err instanceof Error ? err.message : 'error'}`);
    } finally {
      marking = false;
    }
  }
</script>

<Card class="applied-prompt">
  <p class="question">Did you submit your application?</p>
  <p class="job">{job.title} · {job.company}</p>
  <div class="actions">
    <Button variant="ghost" size="sm" onclick={onDone} disabled={marking}>Not yet</Button>
    <Button variant="primary" size="sm" onclick={confirm} disabled={marking}>
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
