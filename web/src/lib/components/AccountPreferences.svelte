<script lang="ts">
  import AccountLanguage from './AccountLanguage.svelte';
  import AccountTimezone from './AccountTimezone.svelte';

  // Preferences about the ACCOUNT rather than about the candidate the profile
  // describes — which is why they sit apart from the profile form while sharing its
  // page. Grouped in one section: separately boxed, they read as two unrelated
  // features when they are one group of settings, so each row carries only its own
  // heading and control (see AccountTimezone / AccountLanguage).
  //
  // Rendered only by the first-time set-up form, so account settings never wait on a CV
  // upload to become reachable. Once a profile exists they live apart: the timezone in the
  // profile's Location section, the language on /my/account.
  let { class: extra = '' }: { class?: string } = $props();
</script>

<section class="flex flex-col gap-5 {extra}">
  <!-- Same anchor as the Location section's timezone: an account with no profile sees this
       form instead, so the "set your timezone" links must land here too. The two never
       render on one page. -->
  <div id="account-timezone" class="scroll-mt-20">
    <AccountTimezone />
  </div>
  <AccountLanguage />
</section>
