import { type RefObject } from 'react';
import gsap from 'gsap';
import { useGSAP } from '@gsap/react';

gsap.registerPlugin(useGSAP);

export { gsap, useGSAP };

/**
 * Motion rules for the redesign:
 *  - Content is visible by default. Entrances only move things (y), they never
 *    start from opacity 0, so a stalled timeline still leaves the page readable.
 *  - Every animation checks reducedMotion() and does nothing when it is set.
 *  - Short and purposeful: 0.3-0.8s, power2/power3 easing, no bounce.
 */
export function reducedMotion(): boolean {
  try {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  } catch {
    return false;
  }
}

/** Elements marked `data-settle` inside `scope` rise into place once, on mount. */
export function useSettle(scope: RefObject<HTMLElement | null>, dependencies: unknown[] = []) {
  useGSAP(
    () => {
      if (reducedMotion() || !scope.current) return;
      const items = scope.current.querySelectorAll('[data-settle]');
      if (!items.length) return;
      gsap.from(items, { y: 18, duration: 0.7, ease: 'power3.out', stagger: 0.06, clearProps: 'transform' });
    },
    { scope, dependencies },
  );
}

/** A quick, quiet "this changed" cue for content that was swapped in place. */
export function refresh(targets: gsap.TweenTarget) {
  if (reducedMotion()) return;
  // Rise only: content is never faded, so a paused tab cannot leave it half-visible.
  gsap.fromTo(targets, { y: 8 }, { y: 0, duration: 0.4, ease: 'power2.out', stagger: 0.04, clearProps: 'transform' });
}
