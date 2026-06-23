<script lang="ts">
  import { resolve } from '$app/paths';
  import { ApiError, submitJob } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { Submission } from '$lib/types';
  import { Button, Input } from '$lib/ui';

  // Form state. url/title/company are required (the server validates too); the rest
  // are optional. source is the posting's real origin (e.g. an ATS name); left blank
  // it defaults to "manual" server-side.
  let url = $state('');
  let title = $state('');
  let company = $state('');
  let location = $state('');
  let remote = $state(false);
  let description = $state('');
  let source = $state('');

  let submitting = $state(false);
  let formError = $state<string | null>(null);
  // The just-submitted vacancy, shown as a confirmation that it is awaiting review.
  let submitted = $state.raw<Submission | null>(null);

  const canSubmit = $derived(
    url.trim() !== '' && title.trim() !== '' && company.trim() !== '' && !submitting,
  );

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    submitting = true;
    formError = null;
    submitted = null;
    try {
      submitted = await submitJob({
        url: url.trim(),
        title: title.trim(),
        company: company.trim(),
        location: location.trim() || undefined,
        remote,
        description: description.trim() || undefined,
        source: source.trim() || undefined,
      });
      url = title = company = location = description = source = '';
      remote = false;
    } catch (err) {
      // 409 means the URL is already awaiting review; surface the backend message.
      formError =
        err instanceof ApiError ? err.message : 'Could not submit the job. Please try again.';
    } finally {
      submitting = false;
    }
  }
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to submit a job.</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Submit a job</h1>
      <p class="text-sm text-muted-foreground">
        Found a vacancy worth sharing? Submit it for review — a moderator approves it before it
        appears in the catalogue.
      </p>
    </div>

    {#if submitted}
      <div
        class="rounded-lg border border-border bg-secondary/40 p-4 text-sm"
        role="status"
      >
        Thanks — <span class="font-medium">{submitted.title}</span> at
        <span class="font-medium">{submitted.company}</span> was submitted and is awaiting review.
        You can track it under
        <a href={resolve('/my/submissions')} class="underline">My submissions</a>.
      </div>
    {/if}

    <form onsubmit={submit} class="flex flex-col gap-4 rounded-lg border border-border p-4">
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">Job URL <span class="text-destructive">*</span></span>
        <Input bind:value={url} type="url" placeholder="https://…" class="w-full" />
      </label>
      <div class="flex flex-col gap-4 sm:flex-row">
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">Title <span class="text-destructive">*</span></span>
          <Input bind:value={title} placeholder="Senior Go Developer" class="w-full" />
        </label>
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">Company <span class="text-destructive">*</span></span>
          <Input bind:value={company} placeholder="Acme" class="w-full" />
        </label>
      </div>
      <div class="flex flex-col gap-4 sm:flex-row">
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">Location</span>
          <Input bind:value={location} placeholder="Berlin, Germany" class="w-full" />
        </label>
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">Source</span>
          <Input bind:value={source} placeholder="e.g. greenhouse (optional)" class="w-full" />
        </label>
      </div>
      <label class="flex items-center gap-2">
        <input type="checkbox" bind:checked={remote} class="size-4 rounded border-input" />
        <span class="text-sm font-medium">Remote</span>
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">Description</span>
        <textarea
          bind:value={description}
          rows={6}
          placeholder="Paste the job description (optional)."
          class="rounded-lg border border-input bg-transparent px-3 py-2 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30"
        ></textarea>
      </label>

      {#if formError}
        <p class="text-sm text-destructive">{formError}</p>
      {/if}

      <div>
        <Button variant="primary" type="submit" disabled={!canSubmit}>
          {submitting ? 'Submitting…' : 'Submit for review'}
        </Button>
      </div>
    </form>
  </div>
{/if}
