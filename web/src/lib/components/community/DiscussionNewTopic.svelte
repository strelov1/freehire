<script lang="ts">
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { communityFormError } from '$lib/community';
  import { Button, Input } from '$lib/ui';

  let {
    subjectType,
    subjectSlug,
    listPath,
  }: {
    subjectType: string;
    subjectSlug: string;
    /** Route of the thread list this topic belongs to, e.g. "/companies/acme/discussion". */
    listPath: string;
  } = $props();

  let title = $state('');
  let body = $state('');
  let submitting = $state(false);
  let formError = $state<string | null>(null);

  const canSubmit = $derived(title.trim() !== '' && body.trim() !== '' && !submitting);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    submitting = true;
    formError = null;
    try {
      const created = await api.createThread({
        subject_type: subjectType,
        subject_slug: subjectSlug,
        title: title.trim(),
        body: body.trim(),
      });
      // Land on the freshly created thread.
      await goto(`${listPath}/${created.id}`);
    } catch (err) {
      formError = communityFormError(err);
      submitting = false;
    }
  }
</script>

<a class="crumb" href={listPath}>← Back to discussion</a>
<h1 class="title">Start a topic</h1>
<p class="sub">Anonymous — you post under a pseudonym, not your name.</p>

{#if isAuthenticated()}
  <form class="topic-form" onsubmit={submit}>
    <Input bind:value={title} placeholder="A short title" maxlength={140} />
    <textarea bind:value={body} rows="6" placeholder="Say more…" class="topic-form__body"></textarea>
    {#if formError}<p class="topic-form__error">{formError}</p>{/if}
    <div class="topic-form__actions">
      <Button variant="ghost" href={listPath}>Cancel</Button>
      <Button type="submit" disabled={!canSubmit}>{submitting ? 'Posting…' : 'Post topic'}</Button>
    </div>
  </form>
{:else}
  <p class="signin-hint">
    <button class="linklike" onclick={() => openAuthDialog()}>Sign in</button> to start a topic.
  </p>
{/if}

<style>
  .crumb {
    display: inline-block;
    margin-bottom: 1rem;
    color: var(--muted-foreground, #6b7280);
    text-decoration: none;
    font-size: 0.85rem;
  }
  .title {
    margin: 0;
    font-size: 1.4rem;
  }
  .sub {
    margin: 0.25rem 0 1.25rem;
    color: var(--muted-foreground, #6b7280);
    font-size: 0.85rem;
  }
  .topic-form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .topic-form__body {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--border, #d1d5db);
    border-radius: 0.5rem;
    font: inherit;
    resize: vertical;
  }
  .topic-form__actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }
  .topic-form__error {
    color: var(--destructive, #dc2626);
    font-size: 0.85rem;
    margin: 0;
  }
  .signin-hint {
    color: var(--muted-foreground, #6b7280);
    font-size: 0.9rem;
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    color: var(--primary, #2563eb);
    cursor: pointer;
    font: inherit;
    text-decoration: underline;
  }
</style>
