/**
 * @author: @dorianbaffier
 * @description: Switch Button
 * @version: 2.0.0
 * @date: 2026-10-16
 * @license: MIT
 * @website: https://kokonutui.com
 * @github: https://github.com/kokonut-labs/kokonutui
 *
 * Adapted for Blunt Code (Tailwind 3, data-theme):
 *  - controlled only: `theme` + `onThemeChange` are required, and the root's
 *    `data-theme` is set synchronously so the View Transition snapshots the
 *    new theme (useTheme then applies the same value);
 *  - icon-only; the label is the caller's aria-label/title;
 *  - colours come from the caller's className (app tokens), not zinc;
 *  - the sun/moon parts' light/dark looks live in css/playful.css as plain
 *    CSS individual-transform properties (`scale`, `translate`) — the
 *    original's Tailwind 4 `dark:` utilities don't exist on Tailwind 3;
 *  - jsdom / old-browser guards for `matchMedia` and `Element.animate`.
 * Reduced motion or no View Transitions API: the theme switches instantly.
 */

import { useId, useRef } from 'react';
import type * as React from 'react';
import { flushSync } from 'react-dom';
import { cn } from '../../lib/utils';

type Theme = 'light' | 'dark';

interface SwitchButtonProps extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'onClick'> {
  theme: Theme;
  onThemeChange: (theme: Theme) => void;
  /** Spread the new theme from the button as a circle. */
  reveal?: boolean;
  iconSize?: number;
}

const EASE_OUT = 'cubic-bezier(0.23, 1, 0.32, 1)';
const EASE_IN_OUT = 'cubic-bezier(0.65, 0, 0.35, 1)';
const EASE_REVEAL = 'cubic-bezier(0.4, 0, 0.2, 1)';
const REVEAL_ATTR = 'data-theme-reveal';
const REVEAL_STYLE_ID = 'kokonut-theme-reveal';
const REVEAL_CSS = `html[${REVEAL_ATTR}]::view-transition-old(root),html[${REVEAL_ATTR}]::view-transition-new(root){animation:none;mix-blend-mode:normal}`;

const RAY_ANGLES = [0, 45, 90, 135, 180, 225, 270, 315];
const RAYS = RAY_ANGLES.map((deg) => {
  const rad = (deg * Math.PI) / 180;
  const at = (r: number) => ({ x: Number((12 + r * Math.cos(rad)).toFixed(2)), y: Number((12 + r * Math.sin(rad)).toFixed(2)) });
  return { deg, from: at(7.5), to: at(10) };
});

const ANIMATED_PROPS = ['opacity', 'scale', 'translate', 'filter'] as const;
type Snapshot = Map<Element, Keyframe>;

function snapshot(elements: Element[]): Snapshot {
  const map: Snapshot = new Map();
  for (const el of elements) {
    const style = getComputedStyle(el);
    const frame: Keyframe = { offset: 0 };
    for (const prop of ANIMATED_PROPS) frame[prop] = style.getPropertyValue(prop);
    map.set(el, frame);
    if (typeof el.getAnimations === 'function') for (const animation of el.getAnimations()) animation.cancel();
  }
  return map;
}

function prefersReducedMotion() {
  return typeof window.matchMedia !== 'function' || window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function ensureRevealStyle() {
  if (document.getElementById(REVEAL_STYLE_ID)) return;
  const style = document.createElement('style');
  style.id = REVEAL_STYLE_ID;
  style.textContent = REVEAL_CSS;
  document.head.appendChild(style);
}

function timing(el: Element, toDark: boolean): KeyframeAnimationOptions {
  const { part, index = '0' } = (el as HTMLElement | SVGElement).dataset;
  const i = Number(index);
  switch (part) {
    case 'ray': return { delay: toDark ? i * 18 : 140 + i * 22, duration: 260, easing: EASE_OUT };
    case 'bite': return { delay: toDark ? 60 : 0, duration: 380, easing: EASE_IN_OUT };
    case 'disc': return { delay: toDark ? 40 : 0, duration: 380, easing: EASE_OUT };
    default: return { duration: 240, easing: EASE_OUT };
  }
}

function morph(from: Snapshot, toDark: boolean) {
  if (prefersReducedMotion()) return;
  for (const [el, frame] of from) {
    if (typeof el.animate === 'function') el.animate([frame, {}], { ...timing(el, toDark), fill: 'backwards' });
  }
}

let revealRunning = false;

function revealFrom(button: HTMLElement, apply: () => void) {
  const doc = document as Document & { activeViewTransition?: unknown; startViewTransition?: (cb: () => void) => ViewTransition };
  if (typeof doc.startViewTransition !== 'function' || prefersReducedMotion() || revealRunning || doc.activeViewTransition) {
    apply();
    return;
  }
  const rect = button.getBoundingClientRect();
  const x = rect.left + rect.width / 2;
  const y = rect.top + rect.height / 2;
  const radius = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y));
  const root = document.documentElement;
  ensureRevealStyle();
  root.setAttribute(REVEAL_ATTR, '');
  revealRunning = true;
  let transition: ViewTransition;
  try {
    transition = doc.startViewTransition(() => flushSync(apply));
  } catch {
    root.removeAttribute(REVEAL_ATTR);
    revealRunning = false;
    apply();
    return;
  }
  transition.ready
    .then(() => {
      root.animate({ clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${radius}px at ${x}px ${y}px)`] }, { duration: 500, easing: EASE_REVEAL, pseudoElement: '::view-transition-new(root)' });
    })
    .catch(() => { /* skipped or aborted */ });
  transition.finished
    .catch(() => { /* theme is already applied */ })
    .finally(() => { root.removeAttribute(REVEAL_ATTR); revealRunning = false; });
}

export default function SwitchButton({ className, theme, onThemeChange, reveal = true, iconSize = 16, ...props }: SwitchButtonProps) {
  const maskId = `${useId()}-moon`;
  const iconRef = useRef<SVGSVGElement>(null);

  const handleClick = (e: React.MouseEvent<HTMLButtonElement>) => {
    const next: Theme = theme === 'dark' ? 'light' : 'dark';
    const apply = () => {
      const parts = [...(iconRef.current?.querySelectorAll('[data-part]') ?? [])];
      const before = snapshot(parts);
      document.documentElement.dataset.theme = next;
      onThemeChange(next);
      morph(before, next === 'dark');
    };
    if (reveal) revealFrom(e.currentTarget, apply);
    else apply();
  };

  return (
    <button type="button" aria-pressed={theme === 'dark'} className={cn('theme-switch', className)} {...props} onClick={handleClick}>
      <svg aria-hidden="true" className="pointer-events-none shrink-0 overflow-visible" fill="none" height={iconSize} ref={iconRef} viewBox="0 0 24 24" width={iconSize}>
        <mask id={maskId}>
          <rect fill="white" height="32" width="32" x="-4" y="-4" />
          <circle cx="17" cy="7" data-part="bite" fill="black" r="7.5" />
        </mask>
        <g mask={`url(#${maskId})`}>
          <circle cx="12" cy="12" data-part="disc" fill="currentColor" r="4.75" />
        </g>
        <g stroke="currentColor" strokeLinecap="round" strokeWidth="2">
          {RAYS.map((ray, index) => <line data-index={index} data-part="ray" key={ray.deg} x1={ray.from.x} x2={ray.to.x} y1={ray.from.y} y2={ray.to.y} />)}
        </g>
      </svg>
    </button>
  );
}
