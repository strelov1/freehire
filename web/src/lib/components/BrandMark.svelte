<script lang="ts">
  // The freehire brand mark — a ring enclosing a diamond. `fill="currentColor"`
  // makes it track the theme (dark mark on light, light mark on dark), exactly
  // like the wordmark beside it. Shared by the desktop header (TopBar) and the
  // mobile drawer (HeaderMenu) so the geometry lives in one place. aria-hidden:
  // callers pair it with the visible "freehire" text, which names the link. The
  // OG-image copy in server/og/shared.ts is a separately rendered PNG (satori)
  // and intentionally does not route through here.
  let { class: cls = 'size-5 shrink-0' }: { class?: string } = $props();
</script>

<svg
  viewBox="0 0 512 512"
  class={cls}
  fill="currentColor"
  aria-hidden="true"
  xmlns="http://www.w3.org/2000/svg"
>
  <path
    fill-rule="evenodd"
    clip-rule="evenodd"
    d="M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z"
  />
</svg>
