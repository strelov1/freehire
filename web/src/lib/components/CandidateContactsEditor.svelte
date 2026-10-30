<script lang="ts">
  import { untrack } from 'svelte';
  // The candidate's own contact details, autosaving: a field saves a second after typing
  // stops, or at once when focus leaves the card. Writes go through resumeStore's queue,
  // which builds each save from the block the one before it saved (see saveContacts).
  import type { CandidateContacts } from '#lib/types.js';
  import { Input } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { resumeStore } from '#lib/resume.svelte.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { TextAutosave } from '#lib/textAutosave.js';
  import SaveStatusLine from './SaveStatus.svelte';
  import { messages } from './CandidateContactsEditor.messages';

  const s = $derived(t(messages, locale()));

  let { contacts = {} }: { contacts?: CandidateContacts | null } = $props();

  type Identity = Pick<CandidateContacts, 'full_name' | 'email' | 'phone' | 'location' | 'links'>;

  let fullName = $state(contacts?.full_name ?? '');
  let email = $state(contacts?.email ?? '');
  let phone = $state(contacts?.phone ?? '');
  let location = $state(contacts?.location ?? '');
  let linksText = $state((contacts?.links ?? []).join('\n'));
  let emailInvalid = $state(false);
  const status = new SaveStatus();

  const splitLinks = (text: string) =>
    text
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean);

  function fromFields(): Identity {
    return {
      full_name: fullName.trim(),
      email: email.trim(),
      phone: phone.trim(),
      location: location.trim(),
      links: splitLinks(linksText),
    };
  }

  function fromSaved(c: CandidateContacts | null | undefined): Identity {
    return {
      full_name: c?.full_name ?? '',
      email: c?.email ?? '',
      phone: c?.phone ?? '',
      location: c?.location ?? '',
      links: c?.links ?? [],
    };
  }

  // A deliberately loose check: it only stops an address that could never deliver from
  // being saved over a good one. The server stays the judge of anything stricter.
  const looksLikeEmail = (v: string) => v === '' || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v);

  const autosave = new TextAutosave<Identity>({
    status,
    read: fromFields,
    saved: () => fromSaved(resumeStore.meta?.contacts ?? contacts),
    save: (value) => resumeStore.saveContacts(value),
    valid: (value) => looksLikeEmail(value.email ?? ''),
  });

  // Follow the saved block when it changes elsewhere (an Undo, a CV parse filling an empty
  // field), but only a field whose saved value differs from what the box holds: a save of
  // this card's own text comes back trimmed, and copying that over the box would eat the
  // space just typed before the next word. The boxes are read untracked on purpose: a
  // keystroke must never re-run this, because the browser runs it between the value
  // update and the `oninput` that marks the edit, when it would still look unedited.
  $effect(() => {
    const saved = fromSaved(contacts);
    if (autosave.holding) return;
    const now = untrack(fromFields);
    if (saved.full_name !== now.full_name) fullName = saved.full_name ?? '';
    if (saved.email !== now.email) email = saved.email ?? '';
    if (saved.phone !== now.phone) phone = saved.phone ?? '';
    if (saved.location !== now.location) location = saved.location ?? '';
    if ((saved.links ?? []).join('\n') !== (now.links ?? []).join('\n')) linksText = (saved.links ?? []).join('\n');
  });

  function edited() {
    emailInvalid = false;
    autosave.edited();
  }

  function commit() {
    emailInvalid = !autosave.commit();
  }

  // Typing right before leaving the page still lands, quietly.
  $effect(() => () => {
    autosave.commit(true);
  });
</script>

<section class="flex flex-col gap-4" onfocusout={commit}>
  <div class="flex items-start justify-between gap-3">
    <div class="flex flex-col gap-1">
      <h3 class="text-sm font-semibold">{s.heading}</h3>
      <p class="text-xs text-muted-foreground">
        {s.description}
      </p>
    </div>
    <SaveStatusLine {status} />
  </div>

  <div class="grid gap-3 sm:grid-cols-2">
    <Input bind:value={fullName} oninput={edited} placeholder={s.fullNamePlaceholder} aria-label={s.fullNamePlaceholder} class="w-full" />
    <div class="flex flex-col gap-1">
      <Input
        bind:value={email}
        oninput={edited}
        type="email"
        placeholder={s.emailPlaceholder}
        aria-label={s.emailPlaceholder}
        aria-invalid={emailInvalid}
        aria-describedby={emailInvalid ? 'contacts-email-error' : undefined}
        class="w-full {emailInvalid ? 'border-destructive focus-visible:ring-destructive' : ''}"
      />
      {#if emailInvalid}
        <p id="contacts-email-error" class="text-xs text-destructive">{s.invalidEmail}</p>
      {/if}
    </div>
    <Input bind:value={phone} oninput={edited} placeholder={s.phonePlaceholder} aria-label={s.phonePlaceholder} class="w-full" />
    <Input bind:value={location} oninput={edited} placeholder={s.locationPlaceholder} aria-label={s.locationPlaceholder} class="w-full" />
  </div>
  <label class="flex flex-col gap-1 text-sm">
    <span class="text-muted-foreground">{s.linksLabel}</span>
    <textarea
      bind:value={linksText}
      oninput={edited}
      rows="3"
      class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      placeholder={s.linksPlaceholder}
    ></textarea>
  </label>
</section>
