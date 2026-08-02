<script lang="ts">
  import { onMount } from 'svelte';

  let canvas = $state<HTMLCanvasElement | null>(null);

  type B = { x: number; y: number; vx: number; vy: number; r: number; hue: number };

  // vivid purple→pink→blue-ish palette, echoing the sphere bubbles
  const HUES = [268, 290, 315, 232, 210, 340];

  onMount(() => {
    const cv = canvas!;
    const ctx = cv.getContext('2d')!;
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    let W = 0,
      H = 0,
      dpr = Math.min(window.devicePixelRatio || 1, 2);
    let bubbles: B[] = [];
    const mouse = { x: -9999, y: -9999, active: false };

    function resize() {
      W = cv.clientWidth;
      H = cv.clientHeight;
      cv.width = Math.floor(W * dpr);
      cv.height = Math.floor(H * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    function spawn() {
      const area = W * H;
      const n = Math.max(14, Math.min(46, Math.round(area / 26000)));
      bubbles = Array.from({ length: n }, () => {
        const r = 22 + Math.random() * 64;
        return {
          x: Math.random() * W,
          y: Math.random() * H,
          vx: (Math.random() - 0.5) * 0.5,
          vy: (Math.random() - 0.5) * 0.5,
          r,
          hue: HUES[(Math.random() * HUES.length) | 0],
        };
      });
    }

    function step() {
      for (const b of bubbles) {
        // gentle gravity down, buoyancy up (net near-zero → lazy drift)
        b.vy += 0.014; // gravity
        b.vy -= 0.02 * (1 - b.r / 120); // buoyancy: smaller bubbles rise faster

        // mouse repulsion (hover interaction)
        if (mouse.active) {
          const dx = b.x - mouse.x;
          const dy = b.y - mouse.y;
          const d2 = dx * dx + dy * dy;
          const reach = 200 + b.r;
          if (d2 < reach * reach) {
            const d = Math.sqrt(d2) || 1;
            const f = (1 - d / reach) * 1.6;
            b.vx += (dx / d) * f;
            b.vy += (dy / d) * f;
          }
        }

        b.vx *= 0.985; // damping
        b.vy *= 0.985;
        // idle sway so it never fully settles
        b.vx += Math.sin((b.y + b.x) * 0.002) * 0.004;

        b.x += b.vx;
        b.y += b.vy;

        // walls (bounce, keep inside)
        if (b.x < b.r) {
          b.x = b.r;
          b.vx = Math.abs(b.vx) * 0.8;
        } else if (b.x > W - b.r) {
          b.x = W - b.r;
          b.vx = -Math.abs(b.vx) * 0.8;
        }
        if (b.y < b.r) {
          b.y = b.r;
          b.vy = Math.abs(b.vy) * 0.8;
        } else if (b.y > H - b.r) {
          b.y = H - b.r;
          b.vy = -Math.abs(b.vy) * 0.8;
        }
      }

      // soft collisions — the "pool" jostle
      for (let i = 0; i < bubbles.length; i++) {
        for (let j = i + 1; j < bubbles.length; j++) {
          const a = bubbles[i],
            c = bubbles[j];
          const dx = c.x - a.x,
            dy = c.y - a.y;
          const min = a.r + c.r;
          const d2 = dx * dx + dy * dy;
          if (d2 > 0 && d2 < min * min) {
            const d = Math.sqrt(d2);
            const overlap = (min - d) / 2;
            const nx = dx / d,
              ny = dy / d;
            a.x -= nx * overlap;
            a.y -= ny * overlap;
            c.x += nx * overlap;
            c.y += ny * overlap;
            const p = (a.vx - c.vx) * nx + (a.vy - c.vy) * ny;
            a.vx -= p * nx * 0.5;
            a.vy -= p * ny * 0.5;
            c.vx += p * nx * 0.5;
            c.vy += p * ny * 0.5;
          }
        }
      }
    }

    function draw() {
      ctx.clearRect(0, 0, W, H);
      ctx.globalCompositeOperation = 'lighter';
      for (const b of bubbles) {
        const g = ctx.createRadialGradient(
          b.x - b.r * 0.35,
          b.y - b.r * 0.4,
          b.r * 0.08,
          b.x,
          b.y,
          b.r,
        );
        g.addColorStop(0, `hsla(${b.hue - 15}, 95%, 92%, 0.9)`);
        g.addColorStop(0.45, `hsla(${b.hue}, 85%, 62%, 0.8)`);
        g.addColorStop(1, `hsla(${b.hue + 22}, 80%, 46%, 0.55)`);
        ctx.beginPath();
        ctx.arc(b.x, b.y, b.r, 0, Math.PI * 2);
        ctx.fillStyle = g;
        ctx.fill();
      }
      ctx.globalCompositeOperation = 'source-over';
    }

    let raf = 0;
    function loop() {
      step();
      draw();
      raf = requestAnimationFrame(loop);
    }

    function onMove(e: PointerEvent) {
      const rect = cv.getBoundingClientRect();
      mouse.x = e.clientX - rect.left;
      mouse.y = e.clientY - rect.top;
      mouse.active = true;
    }
    function onLeave() {
      mouse.active = false;
    }

    resize();
    spawn();
    if (reduce) {
      draw(); // static frame, no animation
    } else {
      loop();
    }
    const ro = new ResizeObserver(() => {
      resize();
      spawn();
      if (reduce) draw();
    });
    ro.observe(cv);
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerleave', onLeave);

    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerleave', onLeave);
    };
  });
</script>

<canvas class="pool" bind:this={canvas}></canvas>

<style>
  .pool {
    position: fixed;
    inset: 0;
    width: 100%;
    height: 100%;
    display: block;
    z-index: 0;
  }
</style>
