<script lang="ts">
  import { Button, FormField, Input } from '#lib/ui/index.js';
  import CompanyPicker from '#lib/components/CompanyPicker.svelte';
  import PeriodDateInput from '#lib/components/PeriodDateInput.svelte';
  import type { PeriodDate } from '#lib/types.js';

  // The experience bank's "Add experience" form. The fields are the caller's, bound.
  let {
    company = $bindable(),
    role = $bindable(),
    location = $bindable(),
    start = $bindable(),
    end = $bindable(),
    current = $bindable(),
    busy,
    onSave,
    onCancel,
  }: {
    company: string;
    role: string;
    location: string;
    start: PeriodDate | undefined;
    end: PeriodDate | undefined;
    current: boolean;
    busy: boolean;
    onSave: () => void;
    onCancel: () => void;
  } = $props();
</script>

<div class="flex flex-col gap-2 rounded-lg bg-muted/40 p-3">
  <p class="text-sm font-medium">New experience</p>
  <FormField label="Company">
    {#snippet children({ id, describedBy })}
      <CompanyPicker {id} aria-describedby={describedBy} bind:value={company} />
    {/snippet}
  </FormField>
  <div class="flex gap-2">
    <FormField label="Role" class="flex-1">
      {#snippet children({ id, describedBy })}
        <Input {id} aria-describedby={describedBy} bind:value={role} />
      {/snippet}
    </FormField>
    <FormField label="Location" class="flex-1">
      {#snippet children({ id, describedBy })}
        <Input {id} aria-describedby={describedBy} bind:value={location} />
      {/snippet}
    </FormField>
  </div>
  <div class="flex gap-2">
    <PeriodDateInput bind:value={start} placeholder="Start" label="Job start" />
    {#if !current}
      <PeriodDateInput bind:value={end} placeholder="End" label="Job end" />
    {/if}
  </div>
  <label class="flex items-center gap-2 text-sm">
    <input type="checkbox" bind:checked={current} class="h-4 w-4" />
    <span class="text-muted-foreground">I currently work here</span>
  </label>
  <div class="flex gap-2">
    <Button size="sm" variant="primary" disabled={busy || !company.trim()} onclick={onSave}>Save</Button>
    <Button size="sm" variant="ghost" onclick={onCancel}>Cancel</Button>
  </div>
</div>
