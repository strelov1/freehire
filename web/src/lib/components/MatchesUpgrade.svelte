<!-- What the match feed is, for an account without Pro or Ultra: no cards, a checkout. -->
<script lang="ts">
  import ProCheckoutButton from './ProCheckoutButton.svelte';
  import type { PublicPrice } from '#lib/types.js';

  const { price }: { price: PublicPrice | null } = $props();
</script>

<section class="flex flex-col items-center gap-4 py-12 text-center">
  <h2 class="text-xl font-semibold">Your daily job matches</h2>
  <ul class="flex flex-col gap-1 text-sm text-muted-foreground">
    <li>Jobs ranked by how well they fit your profile</li>
    <li>A short reason why each one fits, and what may not</li>
    <li>Save or skip in one tap</li>
  </ul>
  <ProCheckoutButton {price} />
  <p class="text-sm text-muted-foreground">Part of Pro and Ultra</p>
</section>
