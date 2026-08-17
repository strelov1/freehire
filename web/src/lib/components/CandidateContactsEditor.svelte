<script lang="ts">
  import { api } from '$lib/api';
  import type { CandidateContacts } from '$lib/types';
  import { Button, Input } from '$lib/ui';

  let { contacts = {}, onSaved }: { contacts?: CandidateContacts | null; onSaved?: () => void } =
    $props();

  let fullName = $state(contacts?.full_name ?? '');
  let email = $state(contacts?.email ?? '');
  let phone = $state(contacts?.phone ?? '');
  let location = $state(contacts?.location ?? '');
  let linksText = $state((contacts?.links ?? []).join('\n'));
  let busy = $state(false);
  let error = $state<string | null>(null);
  let note = $state<string | null>(null);
  // Set by any keystroke, cleared right before a save request is sent. Reloading the
  // `contacts` prop (e.g. after this component's own save round trip) must not overwrite
  // an edit the owner has not saved yet — that would silently discard what they just
  // typed, contradicting the copy below.
  let dirty = $state(false);

  $effect(() => {
    if (dirty) return;
    fullName = contacts?.full_name ?? '';
    email = contacts?.email ?? '';
    phone = contacts?.phone ?? '';
    location = contacts?.location ?? '';
    linksText = (contacts?.links ?? []).join('\n');
  });

  function markDirty() {
    dirty = true;
  }

  async function save() {
    busy = true;
    error = null;
    note = null;
    // Matches the values about to be sent, as of now — a keystroke during the request
    // (e.g. into another field) re-dirties via markDirty and stays protected.
    dirty = false;
    try {
      const links = linksText
        .split('\n')
        .map((l) => l.trim())
        .filter(Boolean);
      // PUT replaces the whole owned block (identity + headline/summary/languages/
      // certifications) — spread the current one first so editing a contact field here
      // cannot wipe out an edit made in the CV summary section.
      await api.putResumeContacts({
        ...contacts,
        full_name: fullName.trim(),
        email: email.trim(),
        phone: phone.trim(),
        location: location.trim(),
        links,
      });
      note = 'Contacts saved.';
      onSaved?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not save contacts.';
      // The save never landed — these values are still unsaved and must stay protected.
      dirty = true;
    } finally {
      busy = false;
    }
  }

</script>

<section class="flex flex-col gap-4">
  <div class="flex flex-col gap-1">
    <h3 class="text-sm font-semibold">Your contacts</h3>
    <p class="text-xs text-muted-foreground">
      Edit without re-uploading. A new CV parse only fills empty fields — it will not overwrite
      what you typed.
    </p>
  </div>

  <div class="grid gap-3 sm:grid-cols-2">
    <Input bind:value={fullName} oninput={markDirty} placeholder="Full name" class="w-full" />
    <Input bind:value={email} oninput={markDirty} placeholder="Email" class="w-full" />
    <Input bind:value={phone} oninput={markDirty} placeholder="Phone" class="w-full" />
    <Input bind:value={location} oninput={markDirty} placeholder="Location" class="w-full" />
  </div>
  <label class="flex flex-col gap-1 text-sm">
    <span class="text-muted-foreground">Links (one per line)</span>
    <textarea
      bind:value={linksText}
      oninput={markDirty}
      rows="3"
      class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      placeholder="https://…"
    ></textarea>
  </label>

  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" variant="primary" disabled={busy} onclick={save}>Save contacts</Button>
  </div>
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else if note}
    <p class="text-xs text-muted-foreground">{note}</p>
  {/if}
</section>
