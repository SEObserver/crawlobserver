<script>
  import { fmtN } from '../utils.js';

  let { value = 0, duration = 5000, format = fmtN } = $props();

  let span;
  let current = value;
  let animFrame = null;
  let firstRender = true;

  $effect(() => {
    const target = value;
    const start = current;
    const delta = target - start;
    if (!span || delta === 0) {
      current = target;
      return;
    }

    // First change: snap instantly
    if (firstRender) {
      firstRender = false;
      current = target;
      // eslint-disable-next-line svelte/no-dom-manipulating -- rAF animation bypass
      span.textContent = format(target);
      return;
    }

    const t0 = performance.now();
    if (animFrame) cancelAnimationFrame(animFrame);

    function tick(now) {
      const elapsed = now - t0;
      const progress = Math.min(elapsed / duration, 1);
      // ease-out quint — strong deceleration at the end
      const p1 = 1 - progress;
      const eased = 1 - p1 * p1 * p1 * p1 * p1;
      current = Math.round(start + delta * eased);
      // eslint-disable-next-line svelte/no-dom-manipulating -- rAF animation bypass
      span.textContent = format(current);
      if (progress < 1) {
        animFrame = requestAnimationFrame(tick);
      } else {
        current = target;
        // eslint-disable-next-line svelte/no-dom-manipulating -- rAF animation bypass
        span.textContent = format(target);
        animFrame = null;
      }
    }

    animFrame = requestAnimationFrame(tick);

    return () => {
      if (animFrame) cancelAnimationFrame(animFrame);
    };
  });
</script>

<span bind:this={span}>{format(value)}</span>
