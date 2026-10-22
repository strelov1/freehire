<script lang="ts">
  import { onMount } from 'svelte';
  import { SectionLabel } from '#lib/ui/index.js';

  // The opening thesis of the page: before any of the five stages run, this is what
  // stage one actually faces — not one board, a field of them. `sources` drives how
  // many points fly (the field's actual density); `companies` is what the headline
  // number says, since "how many employers" is the more legible scale claim than
  // "how many crawler adapters". Both come from the same live catalogue-scale
  // snapshot /open reads (see +page.server.ts), so neither is ever invented or
  // drifts from what the rest of the site claims.
  //
  // Three.js is dynamically imported inside onMount — never in the initial bundle —
  // and the real numbers render as plain HTML underneath the canvas regardless of
  // whether WebGL, motion, or JavaScript itself is available. The canvas is a pure
  // enhancement layered on top; losing it loses nothing true about the page.
  let {
    sources,
    companies,
  }: { sources: number | null; companies: number | null } = $props();

  const compactNf = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 });
  const companiesLabel = $derived(companies != null ? compactNf.format(companies) : null);

  let container = $state<HTMLDivElement | undefined>(undefined);
  let canvas = $state<HTMLCanvasElement | undefined>(undefined);
  let cleanup: (() => void) | undefined;

  onMount(() => {
    let cancelled = false;

    (async () => {
      const pointCount = sources ?? 200;
      const THREE = await import('three');
      if (cancelled) return;

      // The token's raw value can be any CSS Color 4 syntax (this codebase's dark
      // tokens are oklch()), and a modern canvas fillStyle GETTER can hand the same
      // syntax straight back rather than normalizing it — which THREE.Color cannot
      // parse and silently leaves at its white default. Rasterizing one pixel and
      // reading it back is what actually forces concrete 8-bit sRGB, regardless of
      // the input color space or the browser's serialization quirks.
      const ctx = document.createElement('canvas').getContext('2d');
      const readCssColor = (name: string) => {
        const raw = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
        if (!ctx || !raw) return new THREE.Color(0x5b6f00);
        ctx.fillStyle = raw;
        ctx.fillRect(0, 0, 1, 1);
        const pixel = ctx.getImageData(0, 0, 1, 1).data;
        return new THREE.Color((pixel[0] ?? 91) / 255, (pixel[1] ?? 111) / 255, (pixel[2] ?? 0) / 255);
      };

      const scene = new THREE.Scene();
      const camera = new THREE.PerspectiveCamera(50, 1, 0.1, 100);
      camera.position.set(0, 0.3, 5.2);
      camera.lookAt(0, 0, 0);

      let renderer: InstanceType<typeof THREE.WebGLRenderer>;
      try {
        renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
      } catch {
        return; // No WebGL — the HTML figures underneath already say everything true.
      }
      renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));

      // A field of independent flight paths, nothing more: each point launches on a
      // straight chord between two random points on the outer shell and flies it in
      // a dead straight line — uncorrelated with the core, same distribution as the
      // original static field, just moving. Nothing is ever pulled toward the
      // center; the core's only involvement is the scanning ray below.
      const group = new THREE.Group();
      const FIELD_R = 3.3;
      const positions = new Float32Array(pointCount * 3);
      const velocities = new Float32Array(pointCount * 3);
      const colors = new Float32Array(pointCount * 3);
      const brand = readCssColor('--brand');

      // A plain numeric-index read on a typed array is `number | undefined` under
      // this project's tsconfig; every read here is bounds-safe by construction
      // (i*3(+0..2) never exceeds pointCount*3), so this is purely a type escape
      // hatch, not a real fallback.
      const at = (arr: Float32Array, i: number) => arr[i] ?? 0;
      // BufferGeometry.getAttribute() is typed to return possibly-undefined too, for
      // an attribute name that was in fact just set two lines above every call site.
      const attrOf = (g: InstanceType<typeof THREE.BufferGeometry>, name: string) =>
        g.getAttribute(name) as InstanceType<typeof THREE.BufferAttribute>;

      // Uniform-random point on a unit sphere (rejection-free: this parametrization
      // does not clump at the poles the way naive lat/long sampling would).
      const randomOnSphere = (): [number, number, number] => {
        const cosPhi = Math.random() * 2 - 1;
        const sinPhi = Math.sqrt(1 - cosPhi * cosPhi);
        const theta = Math.random() * Math.PI * 2;
        return [Math.cos(theta) * sinPhi, cosPhi, Math.sin(theta) * sinPhi];
      };

      // (Re)launches point i on a chord from one random point on the outer shell to
      // another — a trajectory that has nothing to do with where the core sits, so
      // whether it happens to pass close is geometry, not aim.
      const spawn = (i: number) => {
        const [sx, sy, sz] = randomOnSphere();
        const px = sx * FIELD_R;
        const py = sy * FIELD_R;
        const pz = sz * FIELD_R;
        positions[i * 3] = px;
        positions[i * 3 + 1] = py;
        positions[i * 3 + 2] = pz;

        const [ex, ey, ez] = randomOnSphere();
        const dx = ex * FIELD_R - px;
        const dy = ey * FIELD_R - py;
        const dz = ez * FIELD_R - pz;
        const len = Math.hypot(dx, dy, dz) || 1;
        const speed = 0.0028 + Math.random() * 0.0032;
        velocities[i * 3] = (dx / len) * speed;
        velocities[i * 3 + 1] = (dy / len) * speed;
        velocities[i * 3 + 2] = (dz / len) * speed;
      };
      for (let i = 0; i < pointCount; i++) spawn(i);

      const geometry = new THREE.BufferGeometry();
      geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
      geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3));
      const material = new THREE.PointsMaterial({
        size: 0.075,
        vertexColors: true,
        transparent: true,
        opacity: 0.95,
        sizeAttenuation: true,
      });
      const points = new THREE.Points(geometry, material);
      group.add(points);
      scene.add(group);

      // The one node nothing ever flies into: a dim, larger halo behind the solid
      // core reads as a glow without a postprocessing bloom pass, and both breathe
      // gently on their own slow cycle (see the animation loop below). Named
      // materials, not `.material` casts at the point of use — Mesh's own typing
      // widens that to `Material | Material[]`.
      const coreColor = readCssColor('--foreground');
      const haloMaterial = new THREE.MeshBasicMaterial({ color: coreColor, transparent: true, opacity: 0.12 });
      const coreMaterial = new THREE.MeshBasicMaterial({ color: coreColor });
      const halo = new THREE.Mesh(new THREE.SphereGeometry(0.13, 24, 24), haloMaterial);
      const core = new THREE.Mesh(new THREE.SphereGeometry(0.06, 24, 24), coreMaterial);
      scene.add(halo);
      scene.add(core);

      // The core scanning: several curved rays, each independently snapping out to
      // one flying point at a time — quick to appear, brief hold, quick to fade —
      // then swinging to the next. Every ray is its own Line (its own geometry and
      // material), all children of `group` so they share the points' own local
      // coordinate frame: each always starts at (0,0,0), the core's local position,
      // and its far end tracks its target's live position every frame. The curve is
      // a quadratic bezier through a control point offset perpendicular to the
      // straight line, sampled into a fixed-length polyline every live frame — an
      // actual arc, not a straight spoke.
      const RAY_COUNT = 5;
      const RAY_SEGMENTS = 14;
      const RAY_CYCLE = 0.045; // phase units per animation tick (~60fps): ~0.4s/target — a scan, not a hold
      const RAY_PEAK = 0.9;

      type RayState = {
        geometry: InstanceType<typeof THREE.BufferGeometry>;
        positions: Float32Array;
        positionAttr: InstanceType<typeof THREE.BufferAttribute>;
        material: InstanceType<typeof THREE.LineBasicMaterial>;
        target: number;
        bow: number;
        phase: number;
      };

      const makeRay = (phase: number): RayState => {
        const rayPositions = new Float32Array((RAY_SEGMENTS + 1) * 3);
        const rayGeometry = new THREE.BufferGeometry();
        rayGeometry.setAttribute('position', new THREE.BufferAttribute(rayPositions, 3));
        const rayMaterial = new THREE.LineBasicMaterial({ color: brand, transparent: true, opacity: 0 });
        group.add(new THREE.Line(rayGeometry, rayMaterial));
        return {
          geometry: rayGeometry,
          positions: rayPositions,
          positionAttr: attrOf(rayGeometry, 'position'),
          material: rayMaterial,
          target: Math.floor(Math.random() * pointCount),
          bow: Math.random() < 0.5 ? -1 : 1,
          phase,
        };
      };
      // Staggered start phases so the rays don't all snap to a new target in the
      // same frame — a scan, not a synchronized blink.
      const rays: RayState[] = Array.from({ length: RAY_COUNT }, (_, i) => makeRay(i / RAY_COUNT));

      const updateRayCurve = (ray: RayState) => {
        const ex = at(positions, ray.target * 3);
        const ey = at(positions, ray.target * 3 + 1);
        const ez = at(positions, ray.target * 3 + 2);
        const len = Math.hypot(ex, ey, ez) || 1;
        const dirx = ex / len;
        const diry = ey / len;
        const dirz = ez / len;
        // A reference axis unlikely to be parallel to the line, so the cross product
        // below never degenerates to zero. Its z-component is always 0, so the cross
        // product's own z-dropping terms are omitted rather than written out as `* 0`.
        const upx = Math.abs(diry) > 0.9 ? 1 : 0;
        const upy = Math.abs(diry) > 0.9 ? 0 : 1;
        let cpx = -dirz * upy;
        let cpy = dirz * upx;
        let cpz = dirx * upy - diry * upx;
        const plen = Math.hypot(cpx, cpy, cpz) || 1;
        cpx /= plen;
        cpy /= plen;
        cpz /= plen;
        const bow = len * 0.35 * ray.bow;
        const ctrlX = ex / 2 + cpx * bow;
        const ctrlY = ey / 2 + cpy * bow;
        const ctrlZ = ez / 2 + cpz * bow;
        const rp = ray.positions;
        for (let s = 0; s <= RAY_SEGMENTS; s++) {
          const t = s / RAY_SEGMENTS;
          const it = 1 - t;
          const w1 = 2 * it * t;
          const w2 = t * t;
          rp[s * 3] = w1 * ctrlX + w2 * ex;
          rp[s * 3 + 1] = w1 * ctrlY + w2 * ey;
          rp[s * 3 + 2] = w1 * ctrlZ + w2 * ez;
        }
        ray.positionAttr.needsUpdate = true;
      };

      // Fades each ray in fast, holds it briefly at full opacity on its target's
      // live position, fades it out fast, then snaps to a newly-picked target — a
      // scan beat, not a lingering beam.
      const stepRays = (dt: number) => {
        for (const ray of rays) {
          ray.phase += RAY_CYCLE * dt;
          if (ray.phase >= 1) {
            ray.phase = 0;
            let next = Math.floor(Math.random() * pointCount);
            if (pointCount > 1 && next === ray.target) next = (next + 1) % pointCount;
            ray.target = next;
            ray.bow = Math.random() < 0.5 ? -1 : 1;
          }
          const p = Math.min(1, ray.phase);
          const envelope = p < 0.25 ? p / 0.25 : p > 0.7 ? (1 - p) / 0.3 : 1;
          ray.material.opacity = envelope * RAY_PEAK;
          updateRayCurve(ray);
        }
      };

      // Advances one point by one tick — a dead straight line, nothing bends it.
      // Once it drifts past FIELD_R outbound it's relaunched on a fresh chord.
      const advance = (i: number, dt: number) => {
        const px = at(positions, i * 3) + at(velocities, i * 3) * dt;
        const py = at(positions, i * 3 + 1) + at(velocities, i * 3 + 1) * dt;
        const pz = at(positions, i * 3 + 2) + at(velocities, i * 3 + 2) * dt;
        positions[i * 3] = px;
        positions[i * 3 + 1] = py;
        positions[i * 3 + 2] = pz;
        if (Math.hypot(px, py, pz) > FIELD_R * 1.15) spawn(i);
      };

      // A fresh spawn() puts every point on the outer shell, which would render as a
      // hollow ring on the very first frame — so each point gets independently
      // fast-forwarded a random distance into its own flight before the first paint.
      for (let i = 0; i < pointCount; i++) {
        const warmup = Math.floor(Math.random() * 220);
        for (let k = 0; k < warmup; k++) advance(i, 1);
      }

      const positionAttr = attrOf(geometry, 'position');
      const colorAttr = attrOf(geometry, 'color');
      const step = (dt: number) => {
        for (let i = 0; i < pointCount; i++) {
          advance(i, dt);
          const dist = Math.hypot(at(positions, i * 3), at(positions, i * 3 + 1), at(positions, i * 3 + 2));
          // A depth cue only — brighter the closer it happens to pass to the core on
          // its own straight line, nothing pulls it there.
          const proximity = 1 - Math.min(1, dist / FIELD_R);
          const bright = 0.55 + proximity * 0.85;
          colors[i * 3] = brand.r * bright;
          colors[i * 3 + 1] = brand.g * bright;
          colors[i * 3 + 2] = brand.b * bright;
        }
        positionAttr.needsUpdate = true;
        colorAttr.needsUpdate = true;
      };
      step(0);

      // The text sits top-left (the canvas mask fades that corner out); push the whole
      // field toward the bottom-right so the core node — the brightest, largest thing
      // here — never sits behind a word regardless of how the box's aspect ratio
      // reflows the text block across breakpoints.
      const fieldOffset = new THREE.Vector3(1.2, -0.15, 0);
      group.position.copy(fieldOffset);
      core.position.copy(fieldOffset);
      halo.position.copy(fieldOffset);

      // This whole block runs from onMount, after bind:this has set container — but it's
      // `let`, so TS won't carry that narrowing into the closures below without a `const`
      // alias to hold onto.
      const el = container;
      if (!el) return;
      const resize = () => {
        const w = el.clientWidth;
        const h = el.clientHeight;
        renderer.setSize(w, h, false);
        camera.aspect = w / Math.max(1, h);
        camera.updateProjectionMatrix();
      };
      resize();
      const ro = new ResizeObserver(resize);
      ro.observe(el);

      // `brand`/`coreMaterial`/`haloMaterial`/each ray's material were all set once
      // from the theme active at mount — flipping the site's light/dark toggle
      // (theme.svelte.ts: a `dark` class on <html>) never touched them, so the
      // scene kept the old theme's colors until a full reload re-ran this whole
      // block. Watching the class directly (rather than importing the theme store)
      // keeps this in sync with whatever actually drives the CSS variables, not
      // with one specific way of getting there.
      const applyThemeColors = () => {
        brand.copy(readCssColor('--brand'));
        const foreground = readCssColor('--foreground');
        coreMaterial.color.copy(foreground);
        haloMaterial.color.copy(foreground);
        for (const ray of rays) ray.material.color.copy(brand);
        if (reduceMotion) renderer.render(scene, camera);
      };
      const themeObserver = new MutationObserver(applyThemeColors);
      themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });

      const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      let raf = 0;
      if (reduceMotion) {
        renderer.render(scene, camera);
      } else {
        let last = performance.now();
        const animate = (now: number) => {
          // Clamped so a backgrounded tab regaining focus doesn't fast-forward the
          // flight by several seconds in one jump.
          const dt = Math.min(3, (now - last) / (1000 / 60));
          last = now;

          step(dt);
          stepRays(dt);

          // A slow, smooth breath — deliberately NOT tied to the rays firing. Five
          // rays cycling every ~0.4s each fire roughly every 80ms; snapping the
          // core's scale on every one of those (the previous version) sawtoothed
          // visibly at that rate instead of reading as a pulse. This is a single
          // gentle sine, low amplitude, its own slow period.
          const breathe = 1 + Math.sin(now / 1600) * 0.05;
          core.scale.setScalar(breathe);
          halo.scale.setScalar(breathe);
          haloMaterial.opacity = 0.12 + Math.sin(now / 1600) * 0.04;

          group.rotation.y += 0.0006 * dt;
          renderer.render(scene, camera);
          raf = requestAnimationFrame(animate);
        };
        raf = requestAnimationFrame(animate);
      }

      cleanup = () => {
        cancelAnimationFrame(raf);
        ro.disconnect();
        themeObserver.disconnect();
        geometry.dispose();
        material.dispose();
        core.geometry.dispose();
        coreMaterial.dispose();
        for (const ray of rays) {
          ray.geometry.dispose();
          ray.material.dispose();
        }
        halo.geometry.dispose();
        haloMaterial.dispose();
        renderer.dispose();
      };
    })();

    return () => {
      cancelled = true;
      cleanup?.();
    };
  });
</script>

<div
  bind:this={container}
  class="relative h-72 overflow-hidden sm:h-80 lg:h-96"
>
  <canvas
    bind:this={canvas}
    class="absolute inset-0 h-full w-full"
    style="mask-image: radial-gradient(140% 140% at 0% 0%, transparent, transparent 25%, black 62%); -webkit-mask-image: radial-gradient(140% 140% at 0% 0%, transparent, transparent 25%, black 62%);"
    aria-hidden="true"
  ></canvas>
  <div class="relative z-10 flex h-full flex-col justify-start gap-2 p-6 sm:p-8">
    <SectionLabel text="where it starts" />
    <p class="font-mono text-4xl font-bold tracking-tight sm:text-5xl">
      {companiesLabel ?? 'thousands of'}
      <span class="text-muted-foreground">companies</span>
    </p>
    <p class="max-w-sm text-sm leading-relaxed text-muted-foreground">
      Every open role traces back to one of them — crawled straight from where they
      actually posted it, not a copy of a copy.
    </p>
  </div>
</div>
