<script lang="ts">
  // The one mount point for confirmTailorDialog.svelte.ts's controller — see there
  // for why this needs to exist as a singleton at all. Sibling of CvRefreshDialog in
  // the root layout: fixed, self-gating, owns no layout space of its own.
  //
  // Shows the deterministic (no-LLM) skill/requirement check for the job before the
  // candidate commits to tailoring their CV against it — always, not just when
  // something looks off, so the check is a habit rather than a surprise.
  import { Check, TriangleAlert } from '@lucide/svelte';
  import { confirmTailorDialog, settleConfirmTailorDialog } from '$lib/confirmTailorDialog.svelte';
  import { partitionBlockers, toneText, missingChipClass } from '$lib/jobMatch';
  import { ConfirmDialog } from '$lib/ui';

  const match = $derived(confirmTailorDialog.match);
  const blockers = $derived(partitionBlockers(match?.blockers));
  const missing = $derived(match?.missing ?? []);
  const matchedCount = $derived(match?.matched.length ?? 0);
  const total = $derived(match?.total ?? 0);
  const pct = $derived(match?.coverage_percent ?? 0);
  const hasGaps = $derived(missing.length > 0 || blockers.unmet.length > 0);

  const title = $derived(
    confirmTailorDialog.jobLabel
      ? `Tailor your CV for ${confirmTailorDialog.jobLabel}?`
      : 'Tailor your CV for this role?',
  );
  const confirmLabel = $derived(hasGaps ? 'Tailor anyway' : 'Tailor my CV');
</script>

<ConfirmDialog
  bind:open={
    () => confirmTailorDialog.open,
    (v) => {
      if (!v) settleConfirmTailorDialog(false);
    }
  }
  {title}
  {confirmLabel}
  onConfirm={() => settleConfirmTailorDialog(true)}
>
  {#if confirmTailorDialog.loading && !match}
    <p class="text-sm text-muted-foreground">Checking your fit for this role…</p>
  {:else if match}
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-1.5">
        <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Skills coverage</p>
        {#if missing.length > 0}
          <div class="flex items-baseline justify-between gap-2">
            <span class="text-xs text-muted-foreground">{matchedCount} of {total} required skills</span>
            <span class="text-sm font-bold tabular-nums">{pct}%</span>
          </div>
          <div class="h-1.5 overflow-hidden rounded bg-secondary">
            <div class="h-full rounded bg-warning" style="width: {pct}%"></div>
          </div>
          <div class="flex flex-wrap gap-1.5">
            {#each missing as skill (skill)}<span class={missingChipClass}>{skill}</span>{/each}
          </div>
        {:else}
          <p class="flex items-center gap-1.5 text-sm font-medium text-brand-strong">
            <Check class="size-4 shrink-0" aria-hidden="true" />You match all {total} required skills
          </p>
        {/if}
      </div>

      {#if blockers.unmet.length || blockers.met.length}
        <div class="flex flex-col gap-1.5">
          <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Requirements</p>
          <ul class="flex flex-col gap-1">
            {#each blockers.unmet as b (b.category + b.reason)}
              <li class="flex items-start gap-1.5 text-xs {toneText(b.severity)}">
                <TriangleAlert class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
                <span>{b.reason}</span>
              </li>
            {/each}
            {#each blockers.met as b (b.category + b.reason)}
              <li class="flex items-start gap-1.5 text-xs text-muted-foreground">
                <Check class="mt-0.5 size-3.5 shrink-0 text-brand" aria-hidden="true" />
                <span>{b.reason}</span>
              </li>
            {/each}
          </ul>
        </div>
      {/if}

      <p class="rounded-md bg-muted px-2.5 py-2 text-xs text-muted-foreground">
        Deterministic check against your CV and profile — no AI used, no credit spent yet.
      </p>
    </div>
  {:else}
    <p class="text-sm text-muted-foreground">
      We couldn't check your fit for this role — add a CV to your profile to see it next time.
    </p>
  {/if}
</ConfirmDialog>
