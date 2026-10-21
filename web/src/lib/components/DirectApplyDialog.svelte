<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import type { CvMeta } from '$lib/cv';
  import type { DirectApplyForm } from '$lib/types';
  import { Button, Dialog, FormField } from '$lib/ui';

  // Applying to a vacancy its employer published on freehire. The parent mounts this to
  // open it and unmounts it on onClose; it owns the form. What it sends reaches the
  // employer by email (internal/ingest/directapply), so the consent line before the
  // button names who receives it.
  let {
    slug,
    title,
    onClose,
    onSent,
  }: { slug: string; title: string; onClose: () => void; onSent: () => void } = $props();

  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  let form = $state.raw<DirectApplyForm | null>(null);
  let cvs = $state.raw<CvMeta[]>([]);
  let loadError = $state<string | null>(null);
  let cvId = $state<string | null>(null);
  let answers = $state<Record<string, string>>({});
  let note = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);
  let done = $state(false);

  const fieldClass = 'h-9 w-full rounded-lg border border-input bg-transparent px-3 text-sm';
  const areaClass = 'w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm';

  onMount(async () => {
    try {
      const [f, list] = await Promise.all([api.getDirectApplyForm(slug), api.listCvs()]);
      form = f;
      cvs = list;
      cvId = list[0]?.id ?? null;
      answers = { ...f.prefill };
      if (f.applied) done = true;
    } catch {
      loadError = "Couldn't load the application form. Please try again.";
    }
  });

  const missing = $derived(
    form ? form.questions.filter((q) => q.required && !(answers[q.id] ?? '').trim()).map((q) => q.label) : [],
  );
  const canSubmit = $derived(!!form?.open && cvId !== null && missing.length === 0 && !submitting);

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 400) return e.message;
      if (e.status === 401) return 'Please sign in to apply.';
      if (e.status === 403) return 'Confirm your email address before applying.';
      if (e.status === 404) return 'That CV is no longer available — pick another one.';
      if (e.status === 410) return 'This vacancy no longer takes applications.';
      if (e.status === 429) return "You've reached today's limit of applications.";
      if (e.status === 502) return "The application couldn't be delivered. Please try again.";
    }
    return 'Something went wrong. Please try again.';
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit || cvId === null) return;
    submitting = true;
    error = null;
    try {
      await api.sendDirectApplication(slug, { cv_id: cvId, answers, note: note.trim() || undefined });
      done = true;
      onSent();
    } catch (err) {
      error = messageFor(err);
      // A 409 is either "already sent" or "being sent by another tab"; the form read
      // says which, and only the first is final.
      if (err instanceof ApiError && err.status === 409) {
        const fresh = await api.getDirectApplyForm(slug).catch(() => null);
        if (fresh?.applied) {
          done = true;
          onSent();
        } else {
          error = 'Your application is being sent. Check back in a minute.';
        }
      }
    } finally {
      submitting = false;
    }
  }
</script>

<Dialog bind:open title={`Apply: ${title}`} dismissible={!submitting} class="sm:max-w-lg">
  {#if loadError}
    <p class="text-sm text-destructive">{loadError}</p>
  {:else if !form}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else if done}
    <p class="text-sm">Your application was sent to {form.company_name}. They reply to you by email.</p>
    <div class="mt-4 flex justify-end"><Button onclick={() => (open = false)}>Close</Button></div>
  {:else if !form.open}
    <p class="text-sm text-muted-foreground">This vacancy no longer takes applications.</p>
  {:else}
    <form onsubmit={submit} class="flex flex-col gap-4">
      {#if cvs.length === 0}
        <p class="text-sm">
          You need a CV to apply. <a href={resolve('/my/cvs')} class="underline">Create one</a> and come back.
        </p>
      {:else}
        <FormField label="CV" required>
          {#snippet children({ id, required })}
            <select {id} {required} bind:value={cvId} class={fieldClass}>
              {#each cvs as cv (cv.id)}
                <option value={cv.id}>{cv.title}</option>
              {/each}
            </select>
          {/snippet}
        </FormField>
      {/if}

      {#each form.questions as q (q.id)}
        {#if q.type === 'boolean'}
          <!-- A label names one control; a yes/no question is a group of two, so it is
               named by its legend instead. -->
          <fieldset class="flex flex-col gap-1.5">
            <legend class="text-sm font-medium">
              {q.label}
              {#if q.required}<span class="text-destructive" aria-hidden="true">*</span>{/if}
            </legend>
            <div class="flex gap-4 text-sm">
              {#each [['true', 'Yes'], ['false', 'No']] as [value, label] (value)}
                <label class="flex items-center gap-1.5">
                  <input type="radio" name={q.id} {value} required={q.required} bind:group={answers[q.id]} class="accent-current" />
                  {label}
                </label>
              {/each}
            </div>
          </fieldset>
        {:else}
          <FormField label={q.label} required={q.required}>
            {#snippet children({ id, required })}
              {#if q.type === 'select'}
                <select {id} {required} bind:value={answers[q.id]} class={fieldClass}>
                  <option value="">—</option>
                  {#each q.options ?? [] as option (option)}
                    <option value={option}>{option}</option>
                  {/each}
                </select>
              {:else}
                <textarea {id} {required} bind:value={answers[q.id]} rows="2" maxlength={2000} class={areaClass}></textarea>
              {/if}
            {/snippet}
          </FormField>
        {/if}
      {/each}

      <FormField label="A note to the employer (optional)">
        {#snippet children({ id })}
          <textarea {id} bind:value={note} rows="3" maxlength={2000} class={areaClass}></textarea>
        {/snippet}
      </FormField>

      <p class="text-xs text-muted-foreground">
        {form.company_name} receives your CV, your answers and your email address, and replies to you directly.
      </p>
      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}
      <div class="flex justify-end gap-2">
        <Button type="button" variant="ghost" onclick={() => (open = false)} disabled={submitting}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={!canSubmit}>
          {submitting ? 'Sending…' : 'Send application'}
        </Button>
      </div>
    </form>
  {/if}
</Dialog>
