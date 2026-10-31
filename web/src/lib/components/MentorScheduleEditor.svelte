<script lang="ts">
  import { untrack } from 'svelte';
  import { CalendarOff, Plus, Trash2, X } from '@lucide/svelte';
  import { errorMessage } from '#lib/utils.js';
  import { Button, Card, Input, tabsListVariants, tabsTriggerVariants } from '#lib/ui/index.js';
  import {
    exceptionDayLabel,
    nextWeekRange,
    splitAvailability,
    weekDraft,
    weekRows,
    weekdayLabel,
  } from '#lib/mentorship.js';
  import type { AvailabilityApi, MentorAvailabilityRule } from '#lib/types.js';

  let {
    rules = $bindable(),
    schedule,
    emptyMessage,
  }: {
    rules: MentorAvailabilityRule[];
    /** Where the edits go: the mentor schedule's endpoints or the buddy mock ones. */
    schedule: AvailabilityApi;
    /** What an empty week means on the page hosting the editor. */
    emptyMessage: string;
  } = $props();

  const split = $derived(splitAvailability(rules));

  let saving = $state(false);
  let error = $state('');
  let savedNote = $state(false);

  // The week is edited as a whole and saved as a whole, mirroring the endpoint: a schedule
  // is a shape a mentor reasons about all at once, and a half-applied edit leaves them
  // bookable at hours they just removed. The draft is always on screen; Save lights up
  // only when it differs from what is stored.
  let draft = $state(untrack(() => weekDraft(splitAvailability(rules).weekly)));
  const stored = $derived(JSON.stringify(weekRows(weekDraft(split.weekly))));
  const dirty = $derived(JSON.stringify(weekRows(draft)) !== stored);
  const hasStoredWeek = $derived(split.weekly.length > 0);

  function setDay(index: number, on: boolean) {
    const day = draft[index];
    if (!day) return;
    day.ranges = on ? [nextWeekRange([])] : [];
    savedNote = false;
  }

  function addRange(index: number) {
    const day = draft[index];
    if (!day) return;
    day.ranges = [...day.ranges, nextWeekRange(day.ranges)];
    savedNote = false;
  }

  function removeRange(index: number, at: number) {
    const day = draft[index];
    if (!day) return;
    day.ranges = day.ranges.filter((_, j) => j !== at);
    savedNote = false;
  }

  function discard() {
    draft = weekDraft(split.weekly);
    error = '';
  }

  async function saveWeek() {
    saving = true;
    error = '';
    try {
      await schedule.replaceWeek(weekRows(draft));
      // The PUT answers 204, so the stored rows come from a re-read rather than from the
      // reply. They are worth having: a new row's id is assigned by the server and is what
      // the delete route takes, so a locally-built list would show rows nothing can remove.
      rules = await schedule.reload();
      draft = weekDraft(splitAvailability(rules).weekly);
      savedNote = true;
    } catch (e) {
      error = errorMessage(e, 'The week could not be saved.');
    } finally {
      saving = false;
    }
  }

  let overrideDate = $state('');
  let overrideStart = $state('10:00');
  let overrideEnd = $state('12:00');
  let closing = $state(false);

  async function addOverride() {
    if (!overrideDate) return;
    saving = true;
    error = '';
    try {
      // A closure is an override whose start EQUALS its end. That is the whole trick: one
      // inserted row says "I am away on the 16th", beating the weekly rules for that date,
      // instead of a mentor deleting and rebuilding their week around one absence.
      const end = closing ? overrideStart : overrideEnd;
      const added = await schedule.addOverride(overrideDate, overrideStart, end);
      rules = [...rules, added];
      overrideDate = '';
    } catch (e) {
      error = errorMessage(e, 'The exception could not be added.');
    } finally {
      saving = false;
    }
  }

  async function remove(id: number) {
    saving = true;
    error = '';
    try {
      await schedule.remove(id);
      rules = rules.filter((r) => r.id !== id);
    } catch (e) {
      error = errorMessage(e, 'The exception could not be removed.');
    } finally {
      saving = false;
    }
  }

  const timeClass = 'w-28 tabular-nums';
  const uid = $props.id();
</script>

<Card class="flex flex-col p-0">
  <section class="flex flex-col gap-3 p-5" aria-labelledby="{uid}-weekly">
    <div class="flex flex-col gap-0.5">
      <h2 id="{uid}-weekly" class="text-base font-semibold">Weekly hours</h2>
      <p class="text-sm text-muted-foreground">
        {hasStoredWeek ? 'Repeats every week. Tick a day to open it, then set its hours.' : emptyMessage}
      </p>
    </div>

    <ul class="flex flex-col divide-y divide-border">
      {#each draft as day, i (day.weekday)}
        {@const label = weekdayLabel(day.weekday)}
        {@const open = day.ranges.length > 0}
        <li class="flex flex-wrap items-start gap-x-4 gap-y-1 py-1.5">
          <label class="flex h-9 w-28 shrink-0 items-center gap-2.5 text-sm font-medium sm:w-36">
            <input
              type="checkbox"
              class="size-4 accent-brand"
              checked={open}
              onchange={(e) => setDay(i, e.currentTarget.checked)}
            />
            {label}
          </label>

          {#if open}
            <div class="flex flex-col gap-2">
              {#each day.ranges as range, j (j)}
                <!-- Bound to the range rather than to draft[i].ranges[j]: `$state` makes the
                     draft deeply reactive, so writing through it is the same edit. -->
                <div class="flex items-center gap-2">
                  <Input type="time" bind:value={range.start} class={timeClass} aria-label="{label}, range {j + 1}, from" />
                  <span class="text-sm text-muted-foreground" aria-hidden="true">–</span>
                  <Input type="time" bind:value={range.end} class={timeClass} aria-label="{label}, range {j + 1}, until" />
                  <Button
                    variant="ghost"
                    size="icon"
                    class="text-muted-foreground hover:text-foreground"
                    aria-label="Remove {label} {range.start}–{range.end}"
                    onclick={() => removeRange(i, j)}
                  >
                    <X class="size-4" aria-hidden="true" />
                  </Button>
                  {#if j === day.ranges.length - 1}
                    <Button
                      variant="ghost"
                      size="icon"
                      class="text-muted-foreground hover:text-foreground"
                      aria-label="Add hours on {label}"
                      onclick={() => addRange(i)}
                    >
                      <Plus class="size-4" aria-hidden="true" />
                    </Button>
                  {/if}
                </div>
              {/each}
            </div>
          {:else}
            <span class="flex h-9 items-center text-sm text-muted-foreground">Unavailable</span>
          {/if}
        </li>
      {/each}
    </ul>
  </section>

  <div class="flex flex-wrap items-center gap-2 border-t border-border bg-muted/40 px-5 py-3">
    <Button variant="primary" size="sm" disabled={!dirty || saving} onclick={saveWeek}>
      {saving && dirty ? 'Saving…' : 'Save weekly hours'}
    </Button>
    {#if dirty}
      <Button variant="ghost" size="sm" disabled={saving} onclick={discard}>Discard changes</Button>
      <span class="text-sm text-muted-foreground">Unsaved changes</span>
    {:else if savedNote}
      <span class="text-sm text-muted-foreground" role="status">Saved.</span>
    {/if}
  </div>

  <section class="flex flex-col gap-3 border-t border-border p-5" aria-labelledby="{uid}-exceptions">
    <div class="flex flex-col gap-0.5">
      <h2 id="{uid}-exceptions" class="text-base font-semibold">Date exceptions</h2>
      <p class="text-sm text-muted-foreground">
        A date here replaces your weekly hours for that day entirely — it does not add to them.
      </p>
    </div>

    {#if split.dated.length > 0}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each split.dated as row (row.id)}
          <li class="flex items-center gap-3 py-1.5 pr-1.5 pl-3 text-sm">
            <span class="w-32 shrink-0 font-medium tabular-nums">{exceptionDayLabel(row.date ?? '')}</span>
            <!-- A closure is start === end. Rendering the raw pair would say "10:00–10:00",
                 which nobody would read as "away". -->
            {#if row.closure}
              <span class="flex items-center gap-1.5 text-muted-foreground">
                <CalendarOff class="size-4" aria-hidden="true" /> Away all day
              </span>
            {:else}
              <span class="tabular-nums">{row.start}–{row.end}</span>
            {/if}
            <Button
              variant="ghost"
              size="icon"
              class="ml-auto text-muted-foreground hover:text-destructive"
              aria-label="Remove the exception on {exceptionDayLabel(row.date ?? '')}"
              disabled={saving}
              onclick={() => remove(row.id)}
            >
              <Trash2 class="size-4" aria-hidden="true" />
            </Button>
          </li>
        {/each}
      </ul>
    {/if}

    <div class="flex flex-wrap items-center gap-2 rounded-lg bg-muted/60 p-2">
      <Input type="date" bind:value={overrideDate} class="w-40 tabular-nums" aria-label="Exception date" />
      <div class={tabsListVariants({ class: 'self-auto bg-background' })} role="group" aria-label="Exception kind">
        <button
          type="button"
          class={tabsTriggerVariants({ active: !closing })}
          aria-pressed={!closing}
          onclick={() => (closing = false)}
        >
          Other hours
        </button>
        <button
          type="button"
          class={tabsTriggerVariants({ active: closing })}
          aria-pressed={closing}
          onclick={() => (closing = true)}
        >
          Away all day
        </button>
      </div>
      {#if !closing}
        <span class="flex items-center gap-2">
          <Input type="time" bind:value={overrideStart} class={timeClass} aria-label="Exception from" />
          <span class="text-sm text-muted-foreground" aria-hidden="true">–</span>
          <Input type="time" bind:value={overrideEnd} class={timeClass} aria-label="Exception until" />
        </span>
      {/if}
      <Button
        variant="outline"
        size="sm"
        class="sm:ml-auto"
        disabled={saving || !overrideDate}
        onclick={addOverride}
      >
        <Plus class="size-4" aria-hidden="true" /> Add exception
      </Button>
    </div>
  </section>

  {#if error}
    <p role="alert" class="border-t border-border px-5 py-3 text-sm text-destructive">{error}</p>
  {/if}
</Card>
