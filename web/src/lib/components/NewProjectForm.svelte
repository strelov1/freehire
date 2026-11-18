<script lang="ts">
  import { Button, FormField, Input } from '#lib/ui/index.js';
  import PeriodDateInput from '#lib/components/PeriodDateInput.svelte';
  import type { PeriodDate } from '#lib/types.js';

  // The experience bank's "Add project" form. The fields are the caller's, bound.
  let {
    name = $bindable(),
    link = $bindable(),
    start = $bindable(),
    end = $bindable(),
    busy,
    onSave,
    onCancel,
  }: {
    name: string;
    link: string;
    start: PeriodDate | undefined;
    end: PeriodDate | undefined;
    busy: boolean;
    onSave: () => void;
    onCancel: () => void;
  } = $props();
</script>

<div class="flex flex-col gap-2 rounded-lg bg-muted/40 p-3">
  <p class="text-sm font-medium">New project</p>
  <FormField label="Name">
    {#snippet children({ id, describedBy })}
      <Input {id} aria-describedby={describedBy} bind:value={name} />
    {/snippet}
  </FormField>
  <FormField label="Link" hint="Optional">
    {#snippet children({ id, describedBy })}
      <Input {id} aria-describedby={describedBy} bind:value={link} placeholder="https://…" />
    {/snippet}
  </FormField>
  <div class="flex gap-2">
    <PeriodDateInput bind:value={start} placeholder="Start" label="Project start" />
    <PeriodDateInput bind:value={end} placeholder="End" label="Project end" />
  </div>
  <div class="flex gap-2">
    <Button size="sm" variant="primary" disabled={busy || !name.trim()} onclick={onSave}>Save</Button>
    <Button size="sm" variant="ghost" onclick={onCancel}>Cancel</Button>
  </div>
</div>
