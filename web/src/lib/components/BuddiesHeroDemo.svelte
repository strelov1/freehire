<!-- The buddy feed in miniature for the landing hero, drawn by the real BuddyPost with the
     illustrative DEMO_ITEMS, so it always looks like the product does. `inert`: a picture,
     not a feed — its message and reaction buttons must not be reachable or clickable. -->
<script lang="ts">
  import { DEMO_POSTS } from '#lib/buddiesDemo.js';
  import BuddyPost from './BuddyPost.svelte';
</script>

<figure class="flex min-w-0 flex-col gap-3">
  <p class="sr-only">
    An example feed: a member interviewing at Globex, and another who applied to three jobs today.
  </p>
  <div inert aria-hidden="true" class="flex flex-col gap-3">
    {#each DEMO_POSTS as post (post.key)}
      <BuddyPost {post} />
    {/each}
  </div>
  <figcaption class="text-xs text-muted-foreground">Example feed · handles and companies are made up</figcaption>
</figure>
