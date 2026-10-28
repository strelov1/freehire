<!-- The Pro call to action on a feature landing: straight to checkout at the monthly
     price, the same session /pricing opens. A promo code is entered on /pricing only; a
     discount the account already holds applies here too, since checkout reads it. With
     no price known it falls back to the plan page rather than a dead button. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { priceLabel } from '#lib/proPrice.js';
  import { promptSignIn } from '#lib/signin.js';
  import type { PublicPrice } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';

  const { price, size = 'lg' }: { price: PublicPrice | null; size?: 'lg' | 'md' } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

  async function checkout() {
    if (!price || busy) return;
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    busy = true;
    error = null;
    try {
      const { url } = await api.billingCheckout(price.id);
      window.location.href = url;
    } catch {
      error = 'Checkout is not available right now. Please try again in a moment.';
      busy = false;
    }
  }
</script>

{#if price}
  <Button variant="primary" {size} disabled={busy} onclick={checkout}>
    Get Pro — {priceLabel(price)}/mo
  </Button>
  {#if error}<p class="w-full text-sm text-destructive" role="alert">{error}</p>{/if}
{:else}
  <Button href={resolve('/pricing')} variant="primary" {size}>See Pro plans</Button>
{/if}
