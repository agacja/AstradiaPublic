/* eslint-disable react-refresh/only-export-components -- the glyph path tables and the
   <PlanetGlyph>/<SignGlyph> components are one unit; every screen imports both from here. */
import type { CSSProperties } from 'react';

/**
 * Hand-drawn astrological glyphs — stroke paths in a -10..10 box.
 * Ported from the handoff `tabs_reference/natal-wheel.js` (PPATH / ZPATHS).
 * The design language forbids Unicode/emoji zodiac + planet glyphs anywhere,
 * so every planet/sign symbol in the app renders through these components.
 */

const circ = (cx: number, cy: number, r: number) =>
  'M' + (cx - r) + ',' + cy + ' a' + r + ',' + r + ' 0 1 0 ' + (2 * r) + ',0 a' + r + ',' + r + ' 0 1 0 ' + (-2 * r) + ',0';

export const SIGN_NAMES = ['Aries', 'Taurus', 'Gemini', 'Cancer', 'Leo', 'Virgo', 'Libra', 'Scorpio', 'Sagittarius', 'Capricorn', 'Aquarius', 'Pisces'] as const;

/** Zodiac glyph paths, indexed like SIGN_NAMES (0 = Aries). */
export const SIGN_PATHS: readonly string[] = [
  'M0,7.6 C0,-0.2 -1.1,-3.2 -2.3,-5.5 C-3.5,-7.8 -6.6,-7.3 -6.6,-4.9 M0,7.6 C0,-0.2 1.1,-3.2 2.3,-5.5 C3.5,-7.8 6.6,-7.3 6.6,-4.9',
  circ(0, 2, 5.3) + ' M-6.5,-7.4 C-5.3,-4.7 -3,-3.3 0,-3.3 C3,-3.3 5.3,-4.7 6.5,-7.4',
  'M-6.5,-6.7 C-3,-4.9 3,-4.9 6.5,-6.7 M-6.5,6.7 C-3,4.9 3,4.9 6.5,6.7 M-2.5,-5.3 V5.3 M2.5,-5.3 V5.3',
  circ(-3.5, -2.9, 2.3) + ' ' + circ(3.5, 2.9, 2.3) + ' M-5.8,-2.7 C-5.8,-6 -1.4,-6.8 6,-4.9 M5.8,2.7 C5.8,6 1.4,6.8 -6,4.9',
  circ(-3.9, 3.5, 2.5) + ' M-1.5,2.7 C-3.1,-0.5 -3.7,-3.9 -1.3,-5.9 C1.1,-7.7 4.3,-6.5 4.3,-3.6 C4.3,-0.9 2.1,1.5 2.1,4.3 C2.1,6.3 3.9,6.9 5.5,5.7',
  'M-7.2,-4.4 C-6.4,-6.2 -4.6,-6 -4.6,-3.7 V4.4 M-4.6,-3.7 C-3.8,-6 -1.7,-6.2 -1.7,-3.7 V4.4 M-1.7,-3.7 C-0.9,-6 1.3,-6.2 1.3,-3.7 V4.4 M1.3,-1.5 C3.9,-1.7 5.7,0.4 5.7,2.6 C5.7,5 3.9,6.5 1.3,7.4',
  'M-7,6.6 H7 M-7,3 H-2.4 M2.4,3 H7 M-2.4,3 A4.3,4.3 0 1 1 2.4,3',
  'M-7.2,-4.4 C-6.4,-6.2 -4.6,-6 -4.6,-3.7 V4.4 M-4.6,-3.7 C-3.8,-6 -1.7,-6.2 -1.7,-3.7 V4.4 M-1.7,-3.7 C-0.9,-6 1.3,-6.2 1.3,-3.7 V2.8 C1.3,5.2 2.9,6.6 5.6,6.6 M5.6,6.6 L3.6,4.9 M5.6,6.6 L3.7,8.2',
  'M-6.6,6.6 L6.6,-6.6 M6.6,-6.6 H1 M6.6,-6.6 V-1 M-3.8,-1.2 L1.2,3.8',
  'M-7.2,-5.4 C-6.4,-7 -4.8,-6.7 -4.8,-4.5 V2.6 M-4.8,-4.5 C-4,-6.7 -2,-7 -2,-4.5 V2.8 C-2,5.8 -0.2,7.5 2.1,6.8 C4.7,6 5,2.6 2.9,1.6 C1.1,0.8 -0.7,2.1 -0.7,4.3',
  'M-7,-1.8 L-4.2,-4.4 L-1.4,-1.8 L1.4,-4.4 L4.2,-1.8 L7,-4.4 M-7,3.8 L-4.2,1.2 L-1.4,3.8 L1.4,1.2 L4.2,3.8 L7,1.2',
  'M-4.6,-7.3 C-1.7,-4 -1.7,4 -4.6,7.3 M4.6,-7.3 C1.7,-4 1.7,4 4.6,7.3 M-6.5,0 H6.5',
];

/** Planet glyph paths keyed by planet name. */
export const PLANET_PATHS: Record<string, string> = {
  Sun: circ(0, 0, 6.5) + ' ' + circ(0, 0, 1.1),
  Moon: 'M2.2,-6.8 A7.1,7.1 0 1 0 2.2,6.8 A5.2,5.2 0 1 1 2.2,-6.8',
  Mercury: 'M-3.2,-8 a3.2,3.2 0 0 0 6.4,0 ' + circ(0, -1.8, 3.9) + ' M0,2.1 V7.8 M-2.8,4.9 H2.8',
  Venus: circ(0, -2.8, 4.3) + ' M0,1.5 V8 M-3,4.8 H3',
  Mars: circ(-1.7, 1.7, 4.5) + ' M1.5,-1.5 L6.6,-6.6 M6.6,-6.6 H2.8 M6.6,-6.6 V-2.8',
  Jupiter: 'M-6.4,2.4 H5.2 M2.4,-7 V7 M-6.2,2.4 C-6.2,-2.2 -3.6,-6.4 0.4,-6.8',
  Saturn: 'M-4.4,-7.6 V4.2 M-6.6,-4.6 H-2.2 M-4.4,-0.6 C-1.4,-3.2 2.8,-2 2.8,1.6 C2.8,4.2 1,5.8 -0.6,7.6',
  Uranus: 'M-4.4,-7.2 V0.4 M4.4,-7.2 V0.4 M-4.4,-3.4 H4.4 M0,-3.4 V2.4 ' + circ(0, 4.9, 2.5),
  Neptune: 'M0,-7.4 V6.2 M-3.5,6.2 H3.5 M-5.4,-7 C-5.4,-2.9 -3.1,-1 0,-1 C3.1,-1 5.4,-2.9 5.4,-7',
  Pluto: circ(0, -4.8, 2.1) + ' M-4.5,-4.6 C-4.5,-1 -2.5,0.9 0,0.9 C2.5,0.9 4.5,-1 4.5,-4.6 M0,1.1 V7.4 M-2.8,4.4 H2.8',
  // Lunar nodes / points — simple strokes so nothing falls back to Unicode.
  'North Node': 'M-6,6 C-6,1 -3,-1.5 0,-1.5 C3,-1.5 6,1 6,6 M-6,6 a1.6,1.6 0 1 0 -0.1,0 M6,6 a1.6,1.6 0 1 0 -0.1,0',
  'South Node': 'M-6,-6 C-6,-1 -3,1.5 0,1.5 C3,1.5 6,-1 6,-6 M-6,-6 a1.6,1.6 0 1 0 -0.1,0 M6,-6 a1.6,1.6 0 1 0 -0.1,0',
  Chiron: 'M0,-1 V7.5 M-2.8,4.6 H2.8 M-4.2,-7.5 V-1 M-4.2,-4 L0,-7.5 M-4.2,-4 L0,-0.5 ' + circ(0, -4, 0.01),
  Ascendant: 'M-6,7 L0,-7 L6,7 M-3.5,2 H3.5',
  Midheaven: 'M-6,7 V-7 L0,1 L6,-7 V7',
};

/** Aliases the engine sometimes uses. */
const PLANET_ALIASES: Record<string, string> = {
  sun: 'Sun', moon: 'Moon', mercury: 'Mercury', venus: 'Venus', mars: 'Mars', jupiter: 'Jupiter',
  saturn: 'Saturn', uranus: 'Uranus', neptune: 'Neptune', pluto: 'Pluto',
  NorthNode: 'North Node', SouthNode: 'South Node', Rahu: 'North Node', Ketu: 'South Node',
  ASC: 'Ascendant', Asc: 'Ascendant', MC: 'Midheaven', Mc: 'Midheaven',
};

export function planetPath(name: string): string {
  return PLANET_PATHS[name] ?? PLANET_PATHS[PLANET_ALIASES[name] ?? ''] ?? '';
}

export function signIndex(sign: string): number {
  const i = (SIGN_NAMES as readonly string[]).indexOf(sign);
  if (i >= 0) return i;
  // tolerate lower-case / abbreviated ('Sag', 'Cap')
  const s = sign.toLowerCase();
  return (SIGN_NAMES as readonly string[]).findIndex(n => n.toLowerCase().startsWith(s.slice(0, 3)));
}

export function signPath(sign: string): string {
  const i = signIndex(sign);
  return i >= 0 ? SIGN_PATHS[i] : '';
}

interface GlyphProps {
  size?: number;
  color?: string;
  strokeWidth?: number;
  className?: string;
  style?: CSSProperties;
  title?: string;
}

/** Planet glyph (Sun, Moon, Mercury … Pluto, nodes, ASC/MC). */
export function PlanetGlyph({ name, size = 14, color = '#312e81', strokeWidth = 1.8, className, style, title }: GlyphProps & { name: string }) {
  const d = planetPath(name);
  return (
    <svg width={size} height={size} viewBox="-10 -10 20 20" className={className} style={{ flexShrink: 0, ...style }} aria-label={title ?? name} role="img">
      {title && <title>{title}</title>}
      <path d={d} fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

/** Zodiac sign glyph (by sign name or index). */
export function SignGlyph({ sign, size = 12, color = '#312e81', strokeWidth = 1.8, className, style, title }: GlyphProps & { sign: string | number }) {
  const d = typeof sign === 'number' ? (SIGN_PATHS[((sign % 12) + 12) % 12] ?? '') : signPath(sign);
  const label = typeof sign === 'number' ? SIGN_NAMES[((sign % 12) + 12) % 12] : sign;
  return (
    <svg width={size} height={size} viewBox="-10 -10 20 20" className={className} style={{ flexShrink: 0, ...style }} aria-label={title ?? label} role="img">
      {title && <title>{title}</title>}
      <path d={d} fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

/** Aspect symbols are plain geometry (not emoji) — safe as text, kept here so every screen agrees. */
export const ASPECT_SYMBOLS: Record<string, string> = {
  Conjunction: '☌', Opposition: '☍', Trine: '△', Square: '□', Sextile: '✶',
  conjunction: '☌', opposition: '☍', trine: '△', square: '□', sextile: '✶',
};
