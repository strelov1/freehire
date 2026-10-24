<script lang="ts">
  import { api } from '#lib/api.js';
  import type { CandidateContacts } from '#lib/types.js';
  import { Button, Input } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './CandidateContactsEditor.messages';

  const s = $derived(t(messages, locale()));

  let { contacts = {}, onSaved }: { contacts?: CandidateContacts | null; onSaved?: () => void } =
    $props();

  let fullName = $state(contacts?.full_name ?? '');
  let email = $state(contacts?.email ?? '');
  let phone = $state(contacts?.phone ?? '');
  let location = $state(contacts?.location ?? '');
  let linksText = $state((contacts?.links ?? []).join('\n'));
  let busy = $state(false);
  // What the last save produced, not the display strings themselves — the text is
  // derived from `s` below so it follows a later locale change instead of freezing
  // in whatever locale was resolved when the save settled.
  let saveOutcome = $state<'saved' | 'error' | null>(null);
  // The server's own message when the failure was an Error; `null` means the
  // generic fallback.
  let errorMessage = $state<string | null>(null);
  const note = $derived(saveOutcome === 'saved' ? s.saved : null);
  const error = $derived(saveOutcome === 'error' ? (errorMessage ?? s.saveFailed) : null);
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
    saveOutcome = null;
    errorMessage = null;
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
      saveOutcome = 'saved';
      onSaved?.();
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : null;
      saveOutcome = 'error';
      // The save never landed — these values are still unsaved and must stay protected.
      dirty = true;
    } finally {
      busy = false;
    }
  }

</script>

<section class="flex flex-col gap-4">
  <div class="flex flex-col gap-1">
    <h3 class="text-sm font-semibold">{s.heading}</h3>
    <p class="text-xs text-muted-foreground">
      {s.description}
    </p>
  </div>

  <div class="grid gap-3 sm:grid-cols-2">
    <Input bind:value={fullName} oninput={markDirty} placeholder={s.fullNamePlaceholder} aria-label={s.fullNamePlaceholder} class="w-full" />
    <Input bind:value={email} oninput={markDirty} placeholder={s.emailPlaceholder} aria-label={s.emailPlaceholder} class="w-full" />
    <Input bind:value={phone} oninput={markDirty} placeholder={s.phonePlaceholder} aria-label={s.phonePlaceholder} class="w-full" />
    <Input bind:value={location} oninput={markDirty} placeholder={s.locationPlaceholder} aria-label={s.locationPlaceholder} class="w-full" />
  </div>
  <label class="flex flex-col gap-1 text-sm">
    <span class="text-muted-foreground">{s.linksLabel}</span>
    <textarea
      bind:value={linksText}
      oninput={markDirty}
      rows="3"
      class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      placeholder={s.linksPlaceholder}
    ></textarea>
  </label>

  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" variant="primary" disabled={busy} onclick={save}>{s.save}</Button>
  </div>
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else if note}
    <p class="text-xs text-muted-foreground">{note}</p>
  {/if}
</section>
