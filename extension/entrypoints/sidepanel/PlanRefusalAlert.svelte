<script lang="ts">
  import { Alert, Button } from 'freehire-design-system';
  import { HIRE_ORIGIN } from '../../lib/auth';
  import { track } from '../../lib/analytics';
  import type { PlanRefused } from '../../lib/planRefused';

  /**
   * The plan said no (402): the server's sentence, and the upgrade link when there is
   * a bigger plan to buy. One component for every surface that can be refused — the
   * chat and agent autofill — so a refusal reads and converts the same everywhere.
   */
  let { refusal }: { refusal: PlanRefused } = $props();
</script>

<Alert class="flex flex-col items-start gap-2">
  <span>{refusal.message}</span>
  {#if refusal.upgradeUrl}
    <Button
      variant="primary"
      size="sm"
      href={`${HIRE_ORIGIN}${refusal.upgradeUrl}`}
      target="_blank"
      rel="noreferrer"
      onclick={() => track('upgrade_click', { feature: refusal.feature })}>Upgrade your plan</Button
    >
  {/if}
</Alert>
