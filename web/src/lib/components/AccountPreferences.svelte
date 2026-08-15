<script lang="ts">
  import AccountLanguage from './AccountLanguage.svelte';
  import AccountTimezone from './AccountTimezone.svelte';

  // Preferences about the ACCOUNT rather than about the candidate the profile
  // describes — which is why they sit apart from the profile form while sharing its
  // page. One card holds both: separately boxed, they read as two unrelated features
  // when they are one group of settings, so each row carries only its own heading and
  // control (see AccountTimezone / AccountLanguage) and the card lives here.
  //
  // Rendered in two places — the profile's Settings section and the first-time set-up
  // form, which is that section before there are sections — so account settings never
  // wait on a CV upload to become reachable.
  let { class: extra = '' }: { class?: string } = $props();
</script>

<section class="flex flex-col gap-5 rounded-xl border border-border bg-card p-4 {extra}">
  <AccountTimezone />
  <AccountLanguage />
</section>
