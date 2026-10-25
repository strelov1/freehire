<script lang="ts">
  import { tick } from 'svelte';
  import { AlignLeft, Pencil, Share2, Trash2 } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ApiError } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { signinUrl } from '#lib/signin.js';
  import { jobLists } from '#lib/jobLists.svelte.js';
  import type { JobList } from '#lib/types.js';
  import { Button, ConfirmDialog, Dialog, FormField, Input } from '#lib/ui/index.js';
  import { announce } from '#lib/announce.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import { messages } from './JobListsView.messages';
  import States from './States.svelte';

  const s = $derived(t(messages, locale()));

  // The account page for job lists: create a named list, rename it, edit its
  // description, publish/unpublish it as a public read-only page, and delete it.
  // Adding/removing specific jobs happens from the job card's "Add to list" control,
  // not here — this page manages the lists themselves.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  const items = $derived(jobLists.items);
  // A key into `s.errors`, not the message itself — the text is derived below so
  // an already-shown error follows a later locale change instead of freezing in
  // whatever locale was resolved when it was set. `errorMessage` holds the
  // server's own message when the action's own ApiError carries one; that raw
  // text is never a catalog concern and takes priority over the fallback.
  type ErrorKind = keyof typeof s.errors;
  let errorKind = $state<ErrorKind | null>(null);
  let errorMessage = $state<string | null>(null);
  const error = $derived(errorKind ? (errorMessage ?? s.errors[errorKind]) : null);

  // The error line sits above the list, away from whichever card's button failed, so it is
  // also spoken — at the failure, not from an effect, which a later locale change would
  // re-run and so re-announce an old error.
  function fail(kind: ErrorKind, message: string | null) {
    errorMessage = message;
    errorKind = kind;
    announce(message ?? s.errors[kind], 'assertive');
  }

  async function load() {
    status = 'loading';
    try {
      await jobLists.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  // Load once the session is confirmed; reset the per-user cache on sign-out so a
  // different user does not see the previous one's lists.
  $effect(() => {
    if (isAuthenticated()) {
      void load();
    } else {
      jobLists.reset();
    }
  });

  // Create flow: a small inline form, collapsed by default.
  let creating = $state(false);
  let newName = $state('');
  let newDescription = $state('');
  let createBusy = $state(false);

  function startCreate() {
    creating = true;
    newName = '';
    newDescription = '';
    errorKind = null;
  }

  async function confirmCreate() {
    const name = newName.trim();
    if (!name) return;
    createBusy = true;
    errorKind = null;
    try {
      await jobLists.create(name, newDescription.trim());
      creating = false;
    } catch (err) {
      fail('create', err instanceof ApiError ? err.message : null);
    } finally {
      createBusy = false;
    }
  }

  // Edit flow: one dialog holding both fields, opened on whichever one its button names.
  // It replaced two `window.prompt()`s, which offered no label a screen reader could tie
  // to the field, a single line for a description of up to 2000 characters, and no way to
  // say why an empty name was refused.
  let editTarget = $state<JobList | null>(null);
  let editOpen = $state(false);
  let editName = $state('');
  let editDescription = $state('');
  let editBusy = $state(false);
  let nameInvalid = $state(false);
  // Same split as the page-level error: a key, so a shown message follows the locale.
  let editFailed = $state(false);
  let editFailMessage = $state<string | null>(null);
  const editError = $derived(editFailed ? (editFailMessage ?? s.errors.update) : null);

  // The form, not each field: the name field is the design system's Input, which hands
  // out no element, and both fields are reachable by name from here.
  let editForm = $state<HTMLFormElement | null>(null);
  function focusEditField(field: 'name' | 'description') {
    (editForm?.elements.namedItem(field) as HTMLElement | null)?.focus();
  }

  async function openEdit(l: JobList, field: 'name' | 'description') {
    editTarget = l;
    editName = l.name;
    editDescription = l.description;
    nameInvalid = false;
    editFailed = false;
    editOpen = true;
    // After the dialog has opened, which focuses its own first control.
    await tick();
    focusEditField(field);
  }

  async function saveEdit(event: SubmitEvent) {
    event.preventDefault();
    const l = editTarget;
    if (!l || editBusy) return;
    const name = editName.trim();
    if (!name) {
      nameInvalid = true;
      focusEditField('name');
      return;
    }
    const description = editDescription.trim();
    const patch: { name?: string; description?: string } = {};
    if (name !== l.name) patch.name = name;
    if (description !== l.description) patch.description = description;
    if (Object.keys(patch).length === 0) {
      editOpen = false;
      return;
    }
    editBusy = true;
    editFailed = false;
    try {
      await jobLists.update(l.id, patch);
      editOpen = false;
      announce(s.saved);
    } catch (err) {
      editFailMessage = err instanceof ApiError ? err.message : null;
      editFailed = true;
      announce(editFailMessage ?? s.errors.update, 'assertive');
    } finally {
      editBusy = false;
    }
  }

  let busyId = $state<number | null>(null);
  let copiedId = $state<number | null>(null);

  function listUrl(slug: string): string {
    return `${location.origin}${resolve('/l/[slug]', { slug })}`;
  }

  async function share(id: number) {
    busyId = id;
    errorKind = null;
    try {
      await jobLists.share(id);
    } catch (err) {
      fail('share', err instanceof ApiError ? err.message : null);
    } finally {
      busyId = null;
    }
  }

  async function unshare(id: number) {
    busyId = id;
    errorKind = null;
    try {
      await jobLists.unshare(id);
    } catch {
      fail('unshare', null);
    } finally {
      busyId = null;
    }
  }

  async function copyLink(l: JobList) {
    try {
      await navigator.clipboard.writeText(listUrl(l.public_slug));
      copiedId = l.id;
      announce(s.copied);
      setTimeout(() => {
        if (copiedId === l.id) copiedId = null;
      }, 1500);
    } catch {
      fail('copyLink', null);
    }
  }

  let removeTarget = $state<JobList | null>(null);
  let confirmRemoveOpen = $state(false);

  function requestRemove(l: JobList) {
    removeTarget = l;
    confirmRemoveOpen = true;
  }

  async function remove() {
    const l = removeTarget;
    if (!l) return;
    errorKind = null;
    try {
      await jobLists.remove(l.id);
    } catch {
      fail('delete', null);
    }
  }
</script>

{#if !isAuthenticated()}
  <div class="flex flex-col items-center gap-3 py-12 text-center">
    <p class="text-sm text-muted-foreground">{s.signInPrompt}</p>
    <Button variant="primary" href={signinUrl({ returnTo: page.url.pathname + page.url.search, mode: 'login' })}>{s.signIn}</Button>
  </div>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">{s.heading}</h1>
      <p class="text-sm text-muted-foreground">
        {s.description}
      </p>
    </div>

    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message={s.loadError} />
    {:else}
      {#if creating}
        <div class="flex flex-col gap-2 rounded-xl border border-border p-4">
          <Input bind:value={newName} placeholder={s.namePlaceholder} aria-label={s.namePlaceholder} maxlength={100} />
          <Input bind:value={newDescription} placeholder={s.descriptionPlaceholder} aria-label={s.descriptionPlaceholder} maxlength={2000} />
          <div class="flex items-center gap-2">
            <Button variant="primary" size="sm" disabled={createBusy || !newName.trim()} onclick={confirmCreate}>
              {createBusy ? s.creating : s.create}
            </Button>
            <Button variant="ghost" size="sm" onclick={() => (creating = false)}>{s.cancel}</Button>
          </div>
        </div>
      {:else}
        <Button variant="secondary" size="sm" class="self-start" onclick={startCreate}>{s.newList}</Button>
      {/if}

      {#if items.length === 0}
        <States state="empty" message={s.empty} />
      {:else}
        <div class="flex flex-col gap-3">
        {#each items as l (l.id)}
          <article class="flex flex-col rounded-xl border border-border p-4 transition-colors hover:border-muted-foreground/30">
            <div class="flex items-start gap-3">
              <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                <span class="truncate text-sm font-medium">{l.name}</span>
                <span class="text-xs text-muted-foreground">
                  {format(plural(locale(), l.job_count, s.jobCount), { count: String(l.job_count) })}
                  {#if l.public_slug}· <span class="font-medium text-brand-strong">{s.shared}</span>{/if}
                </span>
                {#if l.description}
                  <span class="mt-1 text-xs text-muted-foreground">{l.description}</span>
                {/if}
              </div>
              <div class="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  aria-label={format(s.renameAriaLabel, { name: l.name })}
                  title={s.renameTitle}
                  onclick={() => openEdit(l, 'name')}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <Pencil class="size-4" />
                </button>
                <button
                  type="button"
                  aria-label={format(s.editDescriptionAriaLabel, { name: l.name })}
                  title={s.editDescriptionTitle}
                  onclick={() => openEdit(l, 'description')}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <AlignLeft class="size-4" />
                </button>
                {#if !l.public_slug}
                  <button
                    type="button"
                    aria-label={format(s.shareAriaLabel, { name: l.name })}
                    title={s.shareTitle}
                    disabled={busyId === l.id}
                    onclick={() => share(l.id)}
                    class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  >
                    <Share2 class="size-4" />
                  </button>
                {/if}
                <button
                  type="button"
                  aria-label={format(s.deleteAriaLabel, { name: l.name })}
                  title={s.deleteTitle}
                  onclick={() => requestRemove(l)}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                >
                  <Trash2 class="size-4" />
                </button>
              </div>
            </div>

            {#if l.public_slug}
              <!-- Shared: the public link, copy, and unshare. -->
              <div class="mt-3 flex flex-wrap items-center gap-2 rounded-lg bg-secondary/50 px-3 py-2">
                <a
                  href={resolve('/l/[slug]', { slug: l.public_slug })}
                  class="min-w-0 truncate text-xs text-brand-strong underline-offset-4 hover:underline"
                >
                  /l/{l.public_slug}
                </a>
                <Button variant="ghost" size="sm" class="ml-auto" onclick={() => copyLink(l)}>
                  {copiedId === l.id ? s.copied : s.copyLink}
                </Button>
                <Button variant="ghost" size="sm" disabled={busyId === l.id} onclick={() => unshare(l.id)}>
                  {s.unshare}
                </Button>
              </div>
            {/if}
          </article>
        {/each}
        </div>
      {/if}
    {/if}
  </div>

  <Dialog bind:open={editOpen} title={s.editDialogTitle} dismissible={!editBusy}>
    <!-- `{#if}` because Dialog renders its children unconditionally: the form would
         otherwise sit in the page whether or not anybody opened it. -->
    {#if editOpen}
    <form bind:this={editForm} class="flex flex-col gap-4" onsubmit={saveEdit} novalidate>
      <FormField label={s.nameLabel} required error={nameInvalid ? s.nameRequired : undefined}>
        {#snippet children({ id, describedBy, invalid })}
          <Input
            {id}
            name="name"
            aria-describedby={describedBy}
            aria-invalid={invalid || undefined}
            required
            maxlength={100}
            bind:value={editName}
            oninput={() => {
              nameInvalid = false;
              editFailed = false;
            }}
          />
        {/snippet}
      </FormField>
      <FormField label={s.descriptionLabel} hint={s.descriptionHint}>
        {#snippet children({ id, describedBy })}
          <textarea
            {id}
            name="description"
            aria-describedby={describedBy}
            rows="4"
            maxlength={2000}
            bind:value={editDescription}
            oninput={() => (editFailed = false)}
            class="w-full resize-y rounded-lg border border-input bg-transparent px-3 py-2 text-sm focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          ></textarea>
        {/snippet}
      </FormField>
      {#if editError}
        <p class="text-sm text-destructive">{editError}</p>
      {/if}
      <div class="flex items-center justify-end gap-2">
        <Button variant="ghost" size="sm" disabled={editBusy} onclick={() => (editOpen = false)}>{s.cancel}</Button>
        <Button type="submit" variant="primary" size="sm" disabled={editBusy}>
          {editBusy ? s.saving : s.save}
        </Button>
      </div>
    </form>
    {/if}
  </Dialog>

  <ConfirmDialog
    bind:open={confirmRemoveOpen}
    title={format(s.deleteDialogTitle, { name: removeTarget?.name ?? '' })}
    confirmLabel={s.deleteTitle}
    variant="destructive"
    onConfirm={remove}
  />
{/if}
