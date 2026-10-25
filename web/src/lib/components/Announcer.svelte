<script lang="ts">
  import { announcement } from '#lib/announce.svelte.js';

  // The two live regions every `announce()` call speaks through. Mounted once, in the root
  // layout, so they exist before any message does — see announce.svelte.ts.
</script>

<div class="sr-only" role="status" aria-live="polite" aria-atomic="true">{announcement('polite')}</div>
<div class="sr-only" role="alert" aria-live="assertive" aria-atomic="true">{announcement('assertive')}</div>
