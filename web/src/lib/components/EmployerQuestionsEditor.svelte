<script lang="ts">
  import { Plus, Trash2 } from '@lucide/svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import type { EmployerQuestion } from '$lib/types';
  import { Button, Input, cn } from '$lib/ui';
  import { messages } from './EmployerView.messages';
  import TokenInput from './facets/TokenInput.svelte';

  // The questions an employer asks with a vacancy that takes applications on freehire.
  // The server validates them (applyform.FreehireForm); the limit here only keeps the add
  // button from offering what the server would refuse.
  const MAX_QUESTIONS = 10;

  let { questions = $bindable() }: { questions: EmployerQuestion[] } = $props();

  const s = $derived(t(messages, locale()));

  const selectClass =
    'h-9 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30';

  function add() {
    questions = [...questions, { label: '', type: 'text', required: false }];
  }
  function remove(i: number) {
    questions = questions.filter((_, j) => j !== i);
  }
  function update(i: number, change: (q: EmployerQuestion) => EmployerQuestion) {
    questions = questions.map((q, j) => (j === i ? change(q) : q));
  }
  function setType(i: number, type: EmployerQuestion['type']) {
    update(i, (q) => ({ ...q, type, options: type === 'select' ? (q.options ?? []) : undefined }));
  }
  function addOption(i: number, value: string) {
    const v = value.trim();
    update(i, (q) => (v && !(q.options ?? []).includes(v) ? { ...q, options: [...(q.options ?? []), v] } : q));
  }
  function removeOption(i: number, value: string) {
    update(i, (q) => ({ ...q, options: (q.options ?? []).filter((o) => o !== value) }));
  }
</script>

<fieldset class="flex flex-col gap-3">
  <legend class="text-sm font-medium">{s.questionsLabel}</legend>
  <p class="text-xs text-muted-foreground">{s.questionsHint}</p>

  <!-- Keyed by position: a question has no identity until the server mints one, and two
       new rows can carry the same (empty) label. -->
  {#each questions as q, i (i)}
    <div class="flex flex-col gap-2 rounded-lg border border-border p-3">
      <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
        <Input
          bind:value={q.label}
          placeholder={s.questionLabelPlaceholder}
          aria-label={s.questionLabelPlaceholder}
          maxlength={300}
          class="w-full sm:flex-1"
        />
        <select
          value={q.type}
          onchange={(e) => setType(i, e.currentTarget.value as EmployerQuestion['type'])}
          aria-label={s.questionTypeLabel}
          class={cn(selectClass, 'sm:w-40')}
        >
          <option value="text">{s.questionTypeText}</option>
          <option value="boolean">{s.questionTypeBoolean}</option>
          <option value="select">{s.questionTypeSelect}</option>
        </select>
        <label class="flex items-center gap-1.5 text-sm">
          <input type="checkbox" bind:checked={q.required} class="size-4 accent-current" />
          {s.questionRequired}
        </label>
        <Button type="button" variant="ghost" size="sm" onclick={() => remove(i)} aria-label={s.removeQuestion}>
          <Trash2 class="size-3.5" />
        </Button>
      </div>
      {#if q.type === 'select'}
        <label class="flex flex-col gap-1">
          <span class="text-xs text-muted-foreground">{s.questionOptions}</span>
          <TokenInput tokens={q.options ?? []} onAdd={(v) => addOption(i, v)} onRemove={(v) => removeOption(i, v)} />
        </label>
      {/if}
    </div>
  {/each}

  {#if questions.length < MAX_QUESTIONS}
    <Button type="button" variant="outline" size="sm" class="w-fit gap-1.5" onclick={add}>
      <Plus class="size-3.5" />
      {s.addQuestion}
    </Button>
  {/if}
</fieldset>
