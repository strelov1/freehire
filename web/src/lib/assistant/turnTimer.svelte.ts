// The running turn's timer for AssistantChat: whole seconds since the turn started and a
// cycling spinner glyph, both reset when no turn is active. Called during component
// initialisation, since it registers the effects that drive it.
export function turnTimer(active: () => boolean) {
  const SPINNER_GLYPHS = ['·', '✢', '✳', '✶', '✻', '✽'] as const;
  let elapsedSec = $state(0);
  let spinnerIdx = $state(0);
  let turnStartedAt = $state<number | null>(null);

  $effect(() => {
    if (active() && turnStartedAt === null) {
      turnStartedAt = Date.now();
      elapsedSec = 0;
      spinnerIdx = 0;
    } else if (!active()) {
      turnStartedAt = null;
      elapsedSec = 0;
    }
  });
  $effect(() => {
    if (!active() || turnStartedAt === null) return;
    const id = setInterval(() => {
      elapsedSec = Math.floor((Date.now() - (turnStartedAt as number)) / 1000);
      spinnerIdx = (spinnerIdx + 1) % SPINNER_GLYPHS.length;
    }, 120);
    return () => clearInterval(id);
  });

  return {
    get elapsedSec() {
      return elapsedSec;
    },
    get glyph(): string | undefined {
      return SPINNER_GLYPHS[spinnerIdx];
    },
  };
}
