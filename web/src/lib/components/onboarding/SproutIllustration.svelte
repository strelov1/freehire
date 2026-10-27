<!-- A sprout coming out of the ground, drawn rather than an emoji, leaning gently from its
     root. Decorative; still for anyone who asked for less motion. -->
<svg viewBox="0 0 120 120" class="h-32 w-32" aria-hidden="true">
  <ellipse cx="60" cy="104" rx="38" ry="7" class="fill-brand-muted" />
  <path d="M30 104c8-6 20-9 30-9s22 3 30 9" class="fill-none stroke-brand/40" stroke-width="2" stroke-linecap="round" />
  <g class="sway">
    <path d="M60 100V58" class="fill-none stroke-brand-strong" stroke-width="3.5" stroke-linecap="round" />
    <path d="M60 74c-4-14-16-22-30-20 2 14 14 22 30 20Z" class="fill-brand/70" />
    <path d="M60 64c4-16 17-25 33-23-2 16-16 25-33 23Z" class="fill-brand" />
    <path d="M60 58c-1-7 1-13 5-17" class="fill-none stroke-brand-strong" stroke-width="2.5" stroke-linecap="round" />
    <circle cx="66" cy="40" r="4" class="fill-brand-strong" />
  </g>
</svg>

<style>
  .sway {
    transform-origin: 60px 100px;
    animation: sway 4.5s ease-in-out infinite;
  }
  @keyframes sway {
    0%,
    100% {
      transform: rotate(-3deg);
    }
    50% {
      transform: rotate(3deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .sway {
      animation: none;
    }
  }
</style>
