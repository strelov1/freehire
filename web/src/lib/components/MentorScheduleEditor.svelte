<script lang="ts">
  import { api } from '$lib/api';
  import { errorMessage } from '$lib/utils';
  import { Badge, Button, Card } from '$lib/ui';
  import { splitAvailability, weekdayLabel, weekdayOrder } from '$lib/mentorship';
  import type { MentorAvailabilityRule } from '$lib/types';

  let { rules = $bindable() }: { rules: MentorAvailabilityRule[] } = $props();

  const split = $derived(splitAvailability(rules));

  let saving = $state(false);
  let error = $state('');

  // The week is edited as a whole and saved as a whole, mirroring the endpoint: a schedule
  // is a shape a mentor reasons about all at once, and a half-applied edit leaves them
  // bookable at hours they just removed.
  let draft = $state<{ weekday: number; start: string; end: string }[]>([]);
  let editingWeek = $state(false);

  function startEditing() {
    draft = split.weekly.map((r) => ({
      weekday: r.weekday as number,
      start: r.start,
      end: r.end,
    }));
    editingWeek = true;
    error = '';
  }

  async function saveWeek() {
    saving = true;
    error = '';
    try {
      await api.replaceWeeklyAvailability(draft);
      // The PUT answers 204, so the stored rows come from a re-read rather than from the
      // reply. They are worth having: a new row's id is assigned by the server and is what
      // the delete route takes, so a locally-built list would show rows nothing can remove.
      rules = await api.myMentorAvailability();
      editingWeek = false;
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
      const added = await api.addAvailabilityOverride(overrideDate, overrideStart, end);
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
      await api.deleteAvailabilityRule(id);
      rules = rules.filter((r) => r.id !== id);
    } catch (e) {
      error = errorMessage(e, 'The rule could not be removed.');
    } finally {
      saving = false;
    }
  }
</script>

<Card class="flex flex-col gap-5 p-4">
  <section class="flex flex-col gap-3">
    <div class="flex items-center justify-between">
      <h2 class="text-sm font-medium">Your usual week</h2>
      {#if !editingWeek}
        <Button variant="ghost" size="sm" onclick={startEditing}>Edit</Button>
      {/if}
    </div>

    {#if editingWeek}
      <div class="flex flex-col gap-2">
        {#each draft as row, i (i)}
          <div class="flex flex-wrap items-center gap-2">
            <!-- Bound to the row rather than to draft[i]: `$state` makes the array's
                 objects deeply reactive, so writing through the row is the same edit and
                 does not need an index the type system cannot prove is in range. -->
            <select
              class="border-input bg-background h-9 rounded-md border px-2 text-sm"
              bind:value={row.weekday}
            >
              {#each weekdayOrder() as weekday (weekday)}
                <option value={weekday}>{weekdayLabel(weekday)}</option>
              {/each}
            </select>
            <input
              type="time"
              bind:value={row.start}
              class="border-input bg-background h-9 rounded-md border px-2 text-sm"
            />
            <span class="text-muted-foreground text-sm">to</span>
            <input
              type="time"
              bind:value={row.end}
              class="border-input bg-background h-9 rounded-md border px-2 text-sm"
            />
            <button
              type="button"
              class="text-muted-foreground hover:text-foreground text-sm underline"
              onclick={() => (draft = draft.filter((_, j) => j !== i))}
            >
              Remove
            </button>
          </div>
        {/each}

        <div class="flex gap-2">
          <Button
            variant="ghost"
            size="sm"
            onclick={() => (draft = [...draft, { weekday: 1, start: '18:00', end: '20:00' }])}
          >
            Add a row
          </Button>
          <Button size="sm" disabled={saving} onclick={saveWeek}>
            {saving ? 'Saving…' : 'Save the week'}
          </Button>
          <Button variant="ghost" size="sm" disabled={saving} onclick={() => (editingWeek = false)}>
            Cancel
          </Button>
        </div>
      </div>
    {:else if split.weekly.length === 0}
      <p class="text-muted-foreground text-sm">
        No hours yet — until you add some, your profile offers nothing to book.
      </p>
    {:else}
      <ul class="flex flex-col gap-1 text-sm">
        {#each split.weekly as row (row.id)}
          <li>
            <span class="font-medium">{weekdayLabel(row.weekday as number)}</span>
            {row.start}–{row.end}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="flex flex-col gap-3 border-t pt-4">
    <h2 class="text-sm font-medium">Exceptions</h2>
    <p class="text-muted-foreground text-sm">
      A date here replaces your usual week for that day entirely — it does not add to it.
    </p>

    {#if split.dated.length > 0}
      <ul class="flex flex-col gap-1 text-sm">
        {#each split.dated as row (row.id)}
          <li class="flex items-center gap-2">
            <span class="font-medium">{row.date}</span>
            <!-- A closure is start === end. Rendering the raw pair would say "10:00–10:00",
                 which nobody would read as "away". -->
            {#if row.closure}
              <Badge variant="secondary">away</Badge>
            {:else}
              <span>{row.start}–{row.end}</span>
            {/if}
            <button
              type="button"
              class="text-muted-foreground hover:text-foreground text-sm underline"
              onclick={() => remove(row.id)}
            >
              Remove
            </button>
          </li>
        {/each}
      </ul>
    {/if}

    <div class="flex flex-wrap items-center gap-2">
      <input
        type="date"
        bind:value={overrideDate}
        class="border-input bg-background h-9 rounded-md border px-2 text-sm"
      />
      <label class="flex items-center gap-1 text-sm">
        <input type="checkbox" bind:checked={closing} />
        Away all day
      </label>
      {#if !closing}
        <input
          type="time"
          bind:value={overrideStart}
          class="border-input bg-background h-9 rounded-md border px-2 text-sm"
        />
        <span class="text-muted-foreground text-sm">to</span>
        <input
          type="time"
          bind:value={overrideEnd}
          class="border-input bg-background h-9 rounded-md border px-2 text-sm"
        />
      {/if}
      <Button size="sm" disabled={saving || !overrideDate} onclick={addOverride}>Add</Button>
    </div>
  </section>

  {#if error}
    <p class="text-destructive text-sm">{error}</p>
  {/if}
</Card>
