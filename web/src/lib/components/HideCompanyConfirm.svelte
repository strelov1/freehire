<script lang="ts">
  // The one confirmation for "Hide company" on Matches, shared by the list and the swipe view.
  // Mount it while a hide is pending; it closes itself through `onclose` either way. A refused
  // write (full list, network) stays in the dialog, so the card and the list are untouched
  // and Hide again is the retry.
  import { untrack } from 'svelte';
  import { ApiError } from '#lib/api-core.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { ConfirmDialog } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { messages } from './HideCompanyConfirm.messages';

  let {
    slug,
    name,
    onclose,
  }: {
    slug: string;
    name: string;
    /** hidden: the exclusion is saved and the feed should reload; cancelled: nothing changed. */
    onclose: (result: 'hidden' | 'cancelled') => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  let open = $state(true);
  let hidden = false;

  async function confirm() {
    try {
      await profileStore.avoidCompany(slug);
    } catch (e) {
      throw new Error(e instanceof ApiError && e.status === 409 ? s.full : s.failed, { cause: e });
    }
    hidden = true;
  }

  $effect(() => {
    if (!open) untrack(() => onclose(hidden ? 'hidden' : 'cancelled'));
  });
</script>

<ConfirmDialog
  bind:open
  title={format(s.title, { company: name || slug })}
  description={s.description}
  confirmLabel={s.confirm}
  onConfirm={confirm}
/>
