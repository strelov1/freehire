<script lang="ts">
  // Copyright (c) 2026 Raj Nandan Sharma. Adapted from Kener's StatusBarCalendar
  // (MIT): https://github.com/rajnandan1/kener
  import { browser } from '$app/env';
  import { themeStore } from '#lib/theme.svelte.js';
  import type { HealthStatus } from '#lib/types.js';

  let {
    days,
    barHeight = 40,
    radius = 8,
  }: {
    days: { day: string; status: HealthStatus | null }[];
    barHeight?: number;
    radius?: number;
  } = $props();

  let canvas = $state<HTMLCanvasElement | null>(null);
  let container = $state<HTMLDivElement | null>(null);
  let tooltipEl = $state<HTMLDivElement | null>(null);
  let canvasWidth = $state(0);
  let hovered = $state<{ index: number; x: number; day: string; status: HealthStatus | null } | null>(null);
  let mounted = $state(false);
  let dpr = $state(1);

  const tooltipStyle = $derived.by(() => {
    if (!hovered || !tooltipEl) {
      return `left: 0px; bottom: ${barHeight + 16}px; opacity: 0;`;
    }
    const tooltipWidth = tooltipEl.offsetWidth;
    const halfTooltip = tooltipWidth / 2;
    const padding = 4;
    let left = hovered.x;
    const minLeft = halfTooltip + padding;
    const maxLeft = canvasWidth - halfTooltip - padding;
    if (left < minLeft) left = minLeft;
    else if (left > maxLeft) left = maxLeft;
    return `left: ${left}px; bottom: ${barHeight + 16}px;`;
  });

  let paletteDark: boolean | null = null;
  let palette: { up: string; down: string; degraded: string; none: string } | null = null;

  function paint(className: string): string {
    const probe = document.createElement('span');
    probe.className = className;
    document.body.append(probe);
    const color = getComputedStyle(probe).backgroundColor;
    probe.remove();
    return color;
  }

  function colors() {
    const dark = themeStore.isDark;
    if (palette && paletteDark === dark) return palette;
    paletteDark = dark;
    palette = {
      up: paint('bg-success'),
      down: paint('bg-destructive'),
      degraded: paint('bg-warning'),
      none: paint('bg-muted'),
    };
    return palette;
  }

  function barFill(status: HealthStatus | null, ink: NonNullable<typeof palette>): string {
    if (status === 'operational') return ink.up;
    if (status === 'degraded') return ink.degraded;
    if (status === 'down') return ink.down;
    return ink.none;
  }

  function barWidth(): number {
    if (days.length === 0 || canvasWidth === 0) return 0;
    return Math.max(1, canvasWidth / days.length);
  }

  function roundedRectPath(
    ctx: CanvasRenderingContext2D,
    x: number,
    y: number,
    width: number,
    height: number,
    r: number,
    roundLeft: boolean,
    roundRight: boolean,
  ) {
    const tl = roundLeft ? r : 0;
    const bl = roundLeft ? r : 0;
    const tr = roundRight ? r : 0;
    const br = roundRight ? r : 0;
    ctx.beginPath();
    ctx.moveTo(x + tl, y);
    ctx.lineTo(x + width - tr, y);
    if (tr) ctx.arcTo(x + width, y, x + width, y + tr, tr);
    else ctx.lineTo(x + width, y);
    ctx.lineTo(x + width, y + height - br);
    if (br) ctx.arcTo(x + width, y + height, x + width - br, y + height, br);
    else ctx.lineTo(x + width, y + height);
    ctx.lineTo(x + bl, y + height);
    if (bl) ctx.arcTo(x, y + height, x, y + height - bl, bl);
    else ctx.lineTo(x, y);
    ctx.lineTo(x, y + tl);
    if (tl) ctx.arcTo(x, y, x + tl, y, tl);
    else ctx.lineTo(x, y);
    ctx.closePath();
  }

  function drawBars(highlightIndex: number | null) {
    if (!canvas || canvasWidth === 0 || !mounted || days.length === 0) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const ink = colors();
    const padding = 4;
    const totalHeight = barHeight + padding * 2;
    const scaledWidth = Math.floor(canvasWidth * dpr);
    const scaledHeight = Math.floor(totalHeight * dpr);
    if (canvas.width !== scaledWidth || canvas.height !== scaledHeight) {
      canvas.width = scaledWidth;
      canvas.height = scaledHeight;
      ctx.scale(dpr, dpr);
    }

    const widthEach = barWidth();
    ctx.clearRect(0, 0, canvasWidth, totalHeight);

    for (let i = 0; i < days.length; i++) {
      const day = days[i];
      if (!day) continue;
      const x = Math.round(i * widthEach);
      const roundedBarWidth = Math.max(0, Math.round((i + 1) * widthEach) - x);

      let scale = 1;
      let opacity = 1;
      if (highlightIndex !== null) {
        if (i === highlightIndex) {
          scale = 1.15;
          opacity = 1;
        } else if (i === highlightIndex - 1 || i === highlightIndex + 1) {
          scale = 1.08;
          opacity = 0.9;
        } else {
          opacity = 0.5;
        }
      }

      ctx.globalAlpha = opacity;
      const scaledBarHeight = barHeight * scale;
      const yOffset = padding + (barHeight - scaledBarHeight) / 2;
      const isFirst = i === 0;
      const isLast = i === days.length - 1;

      ctx.save();
      if (isFirst || isLast) {
        roundedRectPath(ctx, x, yOffset, roundedBarWidth, scaledBarHeight, radius * scale, isFirst, isLast);
        ctx.clip();
      }
      ctx.fillStyle = barFill(day.status, ink);
      ctx.fillRect(x, yOffset, roundedBarWidth, scaledBarHeight);
      ctx.restore();
    }
    ctx.globalAlpha = 1;
  }

  function handleMouseMove(event: MouseEvent) {
    if (!canvas || days.length === 0 || canvasWidth === 0) return;
    const rect = canvas.getBoundingClientRect();
    const mouseX = event.clientX - rect.left;
    const widthEach = barWidth();
    let foundIndex = -1;
    for (let i = 0; i < days.length; i++) {
      const barStart = Math.round(i * widthEach);
      const barEnd = Math.round((i + 1) * widthEach);
      if (mouseX >= barStart && mouseX < barEnd) {
        foundIndex = i;
        break;
      }
    }
    if (foundIndex >= 0) {
      if (hovered?.index === foundIndex) return;
      const data = days[foundIndex];
      if (!data) return;
      hovered = {
        index: foundIndex,
        x: Math.round(foundIndex * widthEach) + Math.round(widthEach) / 2,
        day: data.day,
        status: data.status,
      };
      return;
    }
    if (hovered !== null) hovered = null;
  }

  function handleMouseLeave() {
    hovered = null;
  }

  function statusLabel(status: HealthStatus | null): string {
    if (status === 'down') return 'Down';
    if (status === 'degraded') return 'Degraded';
    if (status === 'operational') return 'Operational';
    return 'No data';
  }

  function statusClass(status: HealthStatus | null): string {
    if (status === 'down') return 'text-destructive-strong';
    if (status === 'degraded') return 'text-warning-strong';
    if (status === 'operational') return 'text-success-strong';
    return 'text-muted-foreground';
  }

  $effect(() => {
    if (!browser) return;
    mounted = true;
    dpr = window.devicePixelRatio || 1;
  });

  $effect(() => {
    if (!browser || !container || typeof ResizeObserver === 'undefined') return;
    canvasWidth = container.clientWidth;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) canvasWidth = entry.contentRect.width;
    });
    observer.observe(container);
    return () => observer.disconnect();
  });

  $effect(() => {
    const width = canvasWidth;
    const data = days;
    const highlight = hovered?.index ?? null;
    void themeStore.isDark;
    if (!browser || !mounted || width <= 0 || data.length === 0) return;
    drawBars(highlight);
  });
</script>

<div class="relative w-full" bind:this={container}>
  <div class="overflow-hidden" style="border-radius: {radius}px;">
    <canvas
      bind:this={canvas}
      style="width: 100%; height: {barHeight + 8}px;"
      onmousemove={handleMouseMove}
      onmouseleave={handleMouseLeave}
      aria-label="Status calendar showing {days.length}-day uptime data"
    ></canvas>
  </div>

  {#if hovered}
    <div
      bind:this={tooltipEl}
      class="pointer-events-none absolute z-20 w-max -translate-x-1/2 rounded-md border bg-popover px-2 py-1 text-xs font-medium whitespace-nowrap text-popover-foreground"
      style={tooltipStyle}
    >
      <span class={statusClass(hovered.status)}>{statusLabel(hovered.status)}</span>
      <span class="text-muted-foreground">@</span>
      {hovered.day}
    </div>
  {/if}
</div>

<style>
  canvas {
    image-rendering: pixelated;
  }
</style>
