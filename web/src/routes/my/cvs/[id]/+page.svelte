<script lang="ts">
  // The address of a CV, whichever kind: both open in the same workspace. A BASE CV opens it in
  // base mode (/tailor/base/<id>); a TAILORED copy resolves its vacancy slug from the tailored
  // list and goes to /tailor/<slug>?cv=<id>. An unknown id (or a load failure) falls back to the
  // CV list.
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { api } from '#lib/api.js';
  import { messages } from './messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  const id = $derived(page.params.id ?? '');
  const s = $derived(t(messages, locale()));

  onMount(async () => {
    try {
      const [bases, items] = await Promise.all([api.listBaseCvs(), api.listCvs()]);
      if (bases.some((b) => b.id === id)) {
        await goto(resolve('/tailor/base/[id]', { id }), { replaceState: true });
        return;
      }
      const match = items.find((cv) => cv.id === id);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to both paths; the rule can't see through the appended ?cv= query
      await goto(match ? `${resolve('/tailor/[slug]', { slug: match.job_slug })}?cv=${id}` : resolve('/my/cvs'), { replaceState: true });
    } catch {
      await goto(resolve('/my/cvs'), { replaceState: true });
    }
  });
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<p class="text-muted-foreground">{s.opening}</p>
