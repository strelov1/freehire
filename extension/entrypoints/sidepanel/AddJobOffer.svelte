<script lang="ts">
  import { Plus } from '@lucide/svelte';
  import { Button, Card } from 'freehire-design-system';

  // The Match tab's offer on a vacancy freehire does not carry yet: add it in one click.
  let { adding, onAdd }: { adding: boolean; onAdd: () => void } = $props();
</script>

<Card class="add-job">
  <Button class="add-job-cta" variant="primary" size="lg" onclick={onAdd} disabled={adding}>
    <Plus class="size-4" />
    {adding ? 'Adding this job…' : 'Add this job in one click'}
  </Button>
  <p class="add-job-hint">See your match score and tailor your CV</p>
</Card>

<style>
  /* :global — Card renders its own root, which carries the design system's scope
   * hash rather than this component's; the same reason .tab-strip and
   * .match-scroll > * are written this way. */
  :global(.add-job) {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 20px 16px;
    text-align: center;
  }

  /* A pill, not the square primary button: this is an invitation on an otherwise
   * empty panel, and the panel's own committed action (Autofill, pinned below) is
   * the squared one — keeping the shapes apart keeps the hierarchy readable. */
  :global(.add-job-cta) {
    border-radius: 9999px;
    white-space: normal;
    max-width: 100%;
  }

  .add-job-hint {
    color: var(--muted-foreground);
    font-size: 13px;
    line-height: 1.4;
  }
</style>
