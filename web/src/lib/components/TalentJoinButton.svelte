<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { signinUrl } from '$lib/signin';
  import { NAV } from '$lib/siteNav';
  import { isTalentNetworkMember } from '$lib/talentMembership';
  import { Button } from '$lib/ui';

  // The way into the Talent Network from the catalogue itself.
  //
  // Until now the only invitation lived on /my/profile (TalentNetworkInvite), which a
  // visitor reads AFTER signing in and going looking. This is the other direction: they
  // are already looking at the catalogue, and the thought "I want to be in this" had
  // nowhere to go.
  //
  // It does not repeat that card's copy — the card explains what joining publishes and is
  // the page that owns the decision. This is a door to it, and the door leads to the
  // PROFILE rather than straight to the membership toggle: joining before there is a CV
  // to read mints a member whose own card 404s (`listed` stays false), so the profile is
  // the right first stop.
  //
  // A member is shown nothing. This is an onboarding control, and it is finished once
  // they are in — the profile card is where they manage it.

  // The destination's glyph comes from NAV rather than being picked here, the same rule
  // siteNav's header states: one page, one mark, however many surfaces draw it.
  const Icon = NAV.talent.icon;

  const profile = resolve('/my/profile');

  // Membership, once the SERVER has answered. `null` while unknown: defaulting to
  // "not a member" would flash "Join" at somebody already in it.
  let member = $state<boolean | null>(null);

  const signedIn = $derived(isAuthenticated());

  // A link either way, not `promptSignIn()`. That helper is for the in-place "sign in to
  // do X" actions, which return the visitor to the page they were on and do not retry the
  // click — here the click IS a navigation, so signing in must end where the button said
  // it leads. `returnTo` is what makes the detour invisible.
  const href = $derived(signedIn ? profile : signinUrl({ returnTo: profile, mode: 'login' }));

  $effect(() => {
    // Signing out invalidates the answer as much as it invalidates the session — leaving
    // the old one behind would decide the next visitor's button on the last one's account.
    if (!signedIn) {
      member = null;
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const setting = await api.getTalentNetwork();
        if (cancelled) return;
        member = isTalentNetworkMember(setting.talent_network_visibility);
      } catch {
        // A failed read leaves the button unrendered rather than showing an error: this
        // is an invitation on a page the visitor came to for something else.
      }
    })();
    return () => {
      cancelled = true;
    };
  });
</script>

<!-- Signed out, the button renders immediately: membership cannot be read without a
     session, so there is nothing to wait for and the answer would only be a 401. -->
{#if !signedIn || member === false}
  <Button variant="primary" {href}>
    <Icon class="size-4" aria-hidden="true" />
    Join the network
  </Button>
{/if}
