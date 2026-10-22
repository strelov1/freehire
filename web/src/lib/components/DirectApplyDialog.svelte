<script lang="ts">
  import { onMount } from 'svelte';
  import { FileUp, Paperclip, Sparkles } from '@lucide/svelte';
  import { api, ApiError, RESUME_MAX_MB } from '#lib/api.js';
  import type { DirectApplyForm } from '#lib/types.js';
  import { Button, Dialog, FormField } from '#lib/ui/index.js';

  // Applying to a vacancy its employer published on freehire. The parent mounts this to
  // open it and unmounts it on onClose; it owns the form. What it sends reaches the
  // employer by email (internal/ingest/directapply), so the consent line before the
  // button names who receives it.
  //
  // It sends one of two CVs and never anything else: the CV the candidate tailored for THIS
  // vacancy, attached by itself when there is one, or the CV file they uploaded to their
  // profile. With neither chosen it offers both ways in — upload a file, or tailor one
  // (onTailor hands back to the page's own tailoring flow).
  let {
    slug,
    title,
    onClose,
    onSent,
    onTailor,
  }: { slug: string; title: string; onClose: () => void; onSent: () => void; onTailor: () => void } = $props();

  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  let form = $state.raw<DirectApplyForm | null>(null);
  let loadError = $state<string | null>(null);
  // Which CV goes out; null until the candidate has one to send.
  let cv = $state<'tailored' | 'resume' | null>(null);
  let uploading = $state(false);
  let uploadError = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let answers = $state<Record<string, string>>({});
  let note = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);
  let done = $state(false);

  const fieldClass = 'h-9 w-full rounded-lg border border-input bg-transparent px-3 text-sm';
  const areaClass = 'w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm';

  onMount(async () => {
    try {
      const f = await api.getDirectApplyForm(slug);
      form = f;
      cv = f.cvs.tailored ? 'tailored' : null;
      answers = { ...f.prefill };
      if (f.applied) done = true;
    } catch {
      loadError = "Couldn't load the application form. Please try again.";
    }
  });

  const missing = $derived(
    form ? form.questions.filter((q) => q.required && !(answers[q.id] ?? '').trim()).map((q) => q.label) : [],
  );
  const canSubmit = $derived(!!form?.open && cv !== null && missing.length === 0 && !submitting);

  // Uploading here is the profile's own upload: the file becomes the candidate's stored CV,
  // the one every later application can send too.
  async function upload(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    uploading = true;
    uploadError = null;
    try {
      await api.extractResumeProfile(file);
      const fresh = await api.getDirectApplyForm(slug);
      if (!fresh.cvs.has_resume) throw new Error('not stored');
      form = fresh;
      cv = 'resume';
    } catch (err) {
      uploadError =
        err instanceof ApiError && err.status === 413 ? err.message : "Couldn't upload your CV. Please try again.";
    } finally {
      uploading = false;
    }
  }

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 400) return e.message;
      if (e.status === 401) return 'Please sign in to apply.';
      if (e.status === 403) return 'Confirm your email address before applying.';
      if (e.status === 404) return 'That CV is no longer available. Upload your CV or tailor one again.';
      if (e.status === 410) return 'This vacancy no longer takes applications.';
      if (e.status === 429) return "You've reached today's limit of applications.";
      if (e.status === 502) return "The application couldn't be delivered. Please try again.";
    }
    return 'Something went wrong. Please try again.';
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit || !form || cv === null) return;
    submitting = true;
    error = null;
    try {
      const choice = cv === 'tailored' && form.cvs.tailored ? { cv_id: form.cvs.tailored.id } : { resume: true as const };
      await api.sendDirectApplication(slug, { ...choice, answers, note: note.trim() || undefined });
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
      <div class="flex flex-col gap-2">
        <span class="text-sm font-medium">CV</span>
        {#if cv === 'tailored' && form.cvs.tailored}
          <p class="flex items-center gap-2 text-sm">
            <Paperclip class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span>Attached: <span class="font-medium">{form.cvs.tailored.title}</span>, tailored for this vacancy</span>
          </p>
          {#if form.cvs.has_resume}
            <button type="button" class="w-fit text-xs underline" onclick={() => (cv = 'resume')}>
              Send my uploaded CV instead
            </button>
          {/if}
        {:else if cv === 'resume'}
          <p class="flex items-center gap-2 text-sm">
            <Paperclip class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span>Attached: <span class="font-medium">your uploaded CV</span></span>
          </p>
          {#if form.cvs.tailored}
            <button type="button" class="w-fit text-xs underline" onclick={() => (cv = 'tailored')}>
              Send the CV tailored for this vacancy instead
            </button>
          {/if}
        {:else}
          <div class="flex flex-col gap-2 sm:flex-row">
            {#if form.cvs.has_resume}
              <Button type="button" variant="outline" class="gap-1.5" onclick={() => (cv = 'resume')}>
                <Paperclip class="size-4" aria-hidden="true" /> Use my uploaded CV
              </Button>
            {:else}
              <Button
                type="button"
                variant="outline"
                class="gap-1.5"
                disabled={uploading}
                onclick={() => fileInput?.click()}
              >
                <FileUp class="size-4" aria-hidden="true" />
                {uploading ? 'Uploading…' : 'Upload my CV'}
              </Button>
              <input
                bind:this={fileInput}
                type="file"
                accept="application/pdf"
                class="hidden"
                onchange={upload}
                aria-label="Upload my CV (PDF, up to {RESUME_MAX_MB} MB)"
              />
            {/if}
            <Button type="button" variant="primary" class="gap-1.5" onclick={onTailor}>
              <Sparkles class="size-4" aria-hidden="true" /> Tailor my CV for this vacancy
            </Button>
          </div>
          {#if uploadError}
            <p class="text-sm text-destructive">{uploadError}</p>
          {/if}
        {/if}
      </div>

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
