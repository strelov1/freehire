<script lang="ts">
  import { buddyAvatarName } from '#lib/buddyPresent.js';
  import { Avatar } from '#lib/ui/index.js';

  // A buddy's picture: initials and a colour derived from the handle alone (see
  // buddyAvatarName), drawn the same everywhere a member appears. Never a photo — a buddy
  // handle is an anonymous identity, and any picture tied to the account, even the blurred
  // Talent Network one, would link the two.
  let { handle, size = 'md', class: className }: { handle: string; size?: 'sm' | 'md' | 'lg'; class?: string } = $props();
</script>

<Avatar name={buddyAvatarName(handle)} {size} class={className} />
