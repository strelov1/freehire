<script lang="ts">
  import { page } from '$app/state';
  import ReferralsLandingView from '$lib/components/ReferralsLandingView.svelte';
  import Seo from '$lib/components/Seo.svelte';

  const canonical = $derived(`${page.url.origin}/referrals`);
</script>

<Seo
  title="Get referred — warm intros over cold applies | freehire"
  description="Ask an employee inside the company to put your name forward — anonymously, for free. freehire's referral marketplace connects job seekers with verified insiders who can refer them, and lets employees offer to refer good people in."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <ReferralsLandingView />
</div>
