<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { communityFormError } from '#lib/community.js';
  import { promptSignIn } from '#lib/signin.js';
  import type { CompanyRef, CompanyRelationKind } from '#lib/types.js';
  import { Button, Dialog } from '#lib/ui/index.js';
  import CompanyPicker from './CompanyPicker.svelte';

  // A company's place among its relatives, and the way to correct it (freehire#3895).
  //
  // A parent and its subsidiaries are separate employers with their own jobs and pages —
  // Novartis Slovenija is not Novartis — so they are linked, never merged. Two slugs that are
  // one employer are the other kind of correction: a moderator merges those, and this page's
  // company then redirects to the one it joined. Either is only PROPOSED here; nothing
  // changes until a moderator approves it.
  let {
    name,
    slug,
    parent,
    subsidiaries,
  }: { name: string; slug: string; parent: CompanyRef | null; subsidiaries: CompanyRef[] } = $props();

  let open = $state(false);
  let kind = $state<CompanyRelationKind>('same');
  let targetText = $state('');
  let target = $state<{ slug: string; name: string } | null>(null);
  let note = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);
  let sent = $state(false);

  const pickedSelf = $derived(target?.slug === slug);
  const canSubmit = $derived(!submitting && target !== null && !pickedSelf);

  function openForm() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    kind = 'same';
    targetText = '';
    target = null;
    note = '';
    error = null;
    sent = false;
    open = true;
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit || !target) return; // `target` is narrowed for the call below
    submitting = true;
    error = null;
    try {
      await api.proposeCompanyRelation(slug, { kind, target_slug: target.slug, note: note.trim() });
      sent = true;
    } catch (err) {
      error = communityFormError(err);
    } finally {
      submitting = false;
    }
  }

  const jobs = (n: number) => `${n} open job${n === 1 ? '' : 's'}`;
</script>

<div class="mt-3 flex flex-col gap-2 text-sm">
  {#if parent}
    <p class="text-muted-foreground">
      Part of
      <a class="font-medium text-primary hover:underline" href={resolve('/companies/[slug]', { slug: parent.slug })}
        >{parent.name}</a
      >
      · {jobs(parent.job_count)}
    </p>
  {/if}
  {#if subsidiaries.length > 0}
    <div class="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-muted-foreground">
      <span>Subsidiaries:</span>
      {#each subsidiaries as s (s.slug)}
        <span>
          <a class="font-medium text-primary hover:underline" href={resolve('/companies/[slug]', { slug: s.slug })}
            >{s.name}</a
          >
          <span class="text-xs">({jobs(s.job_count)})</span>
        </span>
      {/each}
    </div>
  {/if}
  <div>
    <button type="button" class="text-xs text-muted-foreground underline-offset-2 hover:underline" onclick={openForm}>
      Same company listed twice, or part of a group? Suggest a correction
    </button>
  </div>
</div>

<Dialog bind:open title={`Correct ${name}`} class="sm:max-w-md">
  {#if sent}
    <div class="flex flex-col gap-4">
      <p class="text-sm">Thanks. A moderator will review it before anything changes.</p>
      <div class="flex justify-end">
        <Button variant="primary" onclick={() => (open = false)}>Close</Button>
      </div>
    </div>
  {:else}
    <form class="flex flex-col gap-4" onsubmit={submit}>
      <fieldset class="flex flex-col gap-2 text-sm">
        <legend class="mb-1 font-medium">{name} is…</legend>
        <label class="flex items-start gap-2">
          <input type="radio" class="mt-0.5" name="kind" value="same" bind:group={kind} />
          <span>
            the same employer as another company here
            <span class="block text-xs text-muted-foreground">
              Its jobs move to that company, and this page redirects there.
            </span>
          </span>
        </label>
        <label class="flex items-start gap-2">
          <input type="radio" class="mt-0.5" name="kind" value="subsidiary" bind:group={kind} />
          <span>
            a subsidiary or local entity of another company
            <span class="block text-xs text-muted-foreground">
              Both pages stay, and each links to the other.
            </span>
          </span>
        </label>
      </fieldset>

      <label class="flex flex-col gap-1.5 text-sm" for="relation-target">
        <span class="font-medium">{kind === 'same' ? 'The company it is' : 'Its parent company'}</span>
        <CompanyPicker id="relation-target" bind:value={targetText} onSelect={(c) => (target = c)} />
      </label>
      {#if pickedSelf}
        <p class="text-sm text-destructive">Pick a different company than this one.</p>
      {/if}

      <label class="flex flex-col gap-1.5 text-sm">
        <span class="font-medium">How do you know? (optional)</span>
        <textarea
          bind:value={note}
          rows="3"
          maxlength={1000}
          placeholder="A shared website, a registration number, the parent's own site…"
          class="resize-y rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        ></textarea>
      </label>

      {#if error}
        <p role="alert" class="text-sm text-destructive">{error}</p>
      {/if}

      <div class="flex justify-end gap-2">
        <Button type="button" variant="ghost" disabled={submitting} onclick={() => (open = false)}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={!canSubmit}>
          {submitting ? 'Sending…' : 'Send for review'}
        </Button>
      </div>
    </form>
  {/if}
</Dialog>
