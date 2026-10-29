<script lang="ts">
  import { Ban } from '@lucide/svelte';
  import AvoidCard from '#lib/components/profile/AvoidCard.svelte';
  import SkillsCard from '#lib/components/profile/SkillsCard.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { handleSaved } from '../actions';
  import { messages } from '../messages';

  // What the candidate works with, then what they never want to see. Avoid used to be a
  // tab of its own; /my/profile/avoid redirects to the anchor below.
  const s = $derived(t(messages, locale()));
</script>

<SkillsCard onProfileChanged={handleSaved} />

<section id="account-avoid" class="mt-8 flex scroll-mt-20 flex-col gap-4 border-t border-border pt-6">
  <h2 class="flex items-center gap-2 text-base font-semibold">
    <Ban class="size-4.5" />{s.headings.avoid}
  </h2>
  <AvoidCard onProfileChanged={handleSaved} />
</section>
