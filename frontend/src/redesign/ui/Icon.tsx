import { cn } from './cn';

/**
 * Astradia's own icon set: a 20px grid, a 1.6 stroke, round caps and joins.
 * Navigation icons are drawn from the subject (a chart wheel, the ten sefirot,
 * a lunar crescent, the Magician's lemniscate) and each carries one "spark",
 * a small dot that lights gold on the active item. Interface icons have none.
 * Use these instead of an icon pack.
 */
type IconDef = { d: string; spark?: [number, number, number]; fill?: boolean };

const dot = (x: number, y: number, r = 0.85) => `M${x - r} ${y}a${r} ${r} 0 1 0 ${2 * r} 0a${r} ${r} 0 1 0 ${-2 * r} 0`;
const SEFIROT = [[5, 4.6], [15, 4.6], [5, 8.6], [15, 8.6], [5, 12.6], [15, 12.6], [10, 10.6], [10, 14.4], [10, 17.8]] as const;

export const ICONS = {
  /* ── Navigation ─────────────────────────────────────────────────────────── */
  natal: { d: 'M2.8 10a7.2 7.2 0 1 0 14.4 0a7.2 7.2 0 1 0-14.4 0M7 10a3 3 0 1 0 6 0a3 3 0 1 0-6 0M10 2.8V7M10 13v4.2M2.8 10H7M13 10h4.2', spark: [15.09, 4.91, 1.7] },
  kabbalah: { d: SEFIROT.map(([x, y]) => dot(x, y)).join(''), spark: [10, 2.6, 1.9] },
  map: { d: 'M2.8 10a7.2 7.2 0 1 0 14.4 0a7.2 7.2 0 1 0-14.4 0M10 2.8c-3.4 2.4-3.4 12 0 14.4M10 2.8c3.4 2.4 3.4 12 0 14.4M3.3 7.3h13.4M3.3 12.7h13.4', spark: [12.55, 10, 1.7] },
  sky: { d: 'M2.6 15.6h14.8M4.4 15.6a5.6 5.6 0 0 1 11.2 0M5.8 6.6v.01M14.2 5.6v.01M11.6 9.8v.01', spark: [9, 3.8, 1.5] },
  tarot: { d: 'M6.6 2.8h6.8a1.8 1.8 0 0 1 1.8 1.8v10.8a1.8 1.8 0 0 1-1.8 1.8H6.6a1.8 1.8 0 0 1-1.8-1.8V4.6a1.8 1.8 0 0 1 1.8-1.8ZM10 6.6l2.4 3.4-2.4 3.4-2.4-3.4Z', spark: [10, 10, 1.1] },
  calendar: { d: 'M9.98 3.03A7 7 0 1 0 15.66 13.13A5.8 5.8 0 1 1 9.98 3.03Z', spark: [16.2, 4.6, 1.6] },
  wellbeing: { d: 'M10 16.6c-3.4 0-5.9-2-6.4-5 2.2 0 4.5 1 6.4 3 1.9-2 4.2-3 6.4-3-.5 3-3 5-6.4 5ZM10 14.2c-1.5-1.7-2.1-4.2-1.3-7.2.5.3 1 .7 1.3 1.2.3-.5.8-.9 1.3-1.2.8 3 .2 5.5-1.3 7.2Z', spark: [10, 3.9, 1.5] },
  friends: { d: 'M2.6 10a4.8 4.8 0 1 0 9.6 0a4.8 4.8 0 1 0-9.6 0M7.8 10a4.8 4.8 0 1 0 9.6 0a4.8 4.8 0 1 0-9.6 0', spark: [10, 5.97, 1.6] },
  markets: { d: 'M2.8 14.6l4.2-4.4 3.4 2.6 5.4-6.4', spark: [15.8, 6.4, 1.8] },
  crypto: { d: 'M7 3.6v12.8M13 3v11.6M5.6 6.8h2.8v6H5.6ZM11.6 5.2h2.8v5.4h-2.8Z', spark: [13, 2.6, 1.5] },
  commodities: { d: 'M2.8 15.6h14.4M4.2 15.6l1.6-4.6h8.4l1.6 4.6M7.2 11l1.2-3.6h3.2l1.2 3.6', spark: [10, 4.4, 1.6] },
  comparison: { d: 'M10 4v12.2M6 16.2h8M4.2 6.2h11.6M4.2 6.2l-2 4.6c.5 1 1.2 1.4 2 1.4s1.5-.4 2-1.4l-2-4.6ZM15.8 6.2l-2 4.6c.5 1 1.2 1.4 2 1.4s1.5-.4 2-1.4l-2-4.6Z', spark: [10, 3.2, 1.6] },
  custom: { d: 'M3.2 10a6.8 6.8 0 1 0 13.6 0a6.8 6.8 0 1 0-13.6 0M10 7v6M7 10h6', spark: [14.81, 5.19, 1.6] },
  backtest: { d: 'M4.59 7.81A6.4 6.4 0 1 1 4.59 12.19M7.36 6.21L4.59 7.81L3.5 4.8', spark: [10.6, 10, 1.5] },
  politics: { d: 'M5 17V4.2M5 4.6h9.6l-2.2 3.1 2.2 3.1H5', spark: [5, 2.8, 1.5] },
  bot: { d: 'M10 10.5C8.4 8.3 6.8 7.2 5.2 7.2a3.3 3.3 0 0 0 0 6.6c1.6 0 3.2-1.1 4.8-3.3Zm0 0c1.6-2.2 3.2-3.3 4.8-3.3a3.3 3.3 0 0 1 0 6.6c-1.6 0-3.2-1.1-4.8-3.3Z', spark: [10, 4.4, 1.6] },
  trade: { d: 'M6.5 15.5V4.6M3.8 7.3l2.7-2.7 2.7 2.7M13.5 4.5v11M10.8 12.8l2.7 2.7 2.7-2.7', spark: [6.5, 3.4, 1.5] },
  crown: { d: 'M3.6 14.6h12.8M4.4 14.6L3.4 6.9l3.9 3.1L10 5.2l2.7 4.8 3.9-3.1-1 7.7', spark: [10, 5.2, 1.7] },
  sidebar: { d: 'M4.2 3.6h11.6a1.4 1.4 0 0 1 1.4 1.4v10a1.4 1.4 0 0 1-1.4 1.4H4.2a1.4 1.4 0 0 1-1.4-1.4V5a1.4 1.4 0 0 1 1.4-1.4ZM7.8 3.6v12.8' },
  more: { d: 'M3.4 10a1.1 1.1 0 1 0 2.2 0a1.1 1.1 0 1 0-2.2 0M8.9 10a1.1 1.1 0 1 0 2.2 0a1.1 1.1 0 1 0-2.2 0', spark: [15.5, 10, 1.7] },

  /* ── Interface ──────────────────────────────────────────────────────────── */
  chevronDown: { d: 'M5.5 8l4.5 4.5L14.5 8' },
  chevronUp: { d: 'M5.5 12l4.5-4.5 4.5 4.5' },
  chevronRight: { d: 'M8 5.5l4.5 4.5L8 14.5' },
  chevronLeft: { d: 'M12 5.5L7.5 10l4.5 4.5' },
  close: { d: 'M5.5 5.5l9 9M14.5 5.5l-9 9' },
  check: { d: 'M4.6 10.6l3.4 3.4 7.4-8' },
  plus: { d: 'M10 4.5v11M4.5 10h11' },
  minus: { d: 'M4.5 10h11' },
  search: { d: 'M3.6 9a5.4 5.4 0 1 0 10.8 0a5.4 5.4 0 1 0-10.8 0M13 13l3.6 3.6' },
  pin: { d: 'M10 17.4s5.4-5 5.4-9.1A5.4 5.4 0 0 0 4.6 8.3c0 4.1 5.4 9.1 5.4 9.1ZM8 8.3a2 2 0 1 0 4 0a2 2 0 1 0-4 0' },
  clock: { d: 'M2.8 10a7.2 7.2 0 1 0 14.4 0a7.2 7.2 0 1 0-14.4 0M10 6v4.3l2.8 1.8' },
  date: { d: 'M4.2 5.2h11.6a1.4 1.4 0 0 1 1.4 1.4v9a1.4 1.4 0 0 1-1.4 1.4H4.2a1.4 1.4 0 0 1-1.4-1.4v-9a1.4 1.4 0 0 1 1.4-1.4ZM2.8 8.8h14.4M6.6 3v3.2M13.4 3v3.2' },
  copy: { d: 'M7.4 7.4h7.8a1.4 1.4 0 0 1 1.4 1.4v7.8a1.4 1.4 0 0 1-1.4 1.4H7.4a1.4 1.4 0 0 1-1.4-1.4V8.8a1.4 1.4 0 0 1 1.4-1.4ZM13.4 4.8a1.4 1.4 0 0 0-1.4-1.4H4.8a1.4 1.4 0 0 0-1.4 1.4V12a1.4 1.4 0 0 0 1.4 1.4' },
  play: { d: 'M6.6 4.2v11.6l9.2-5.8Z', fill: true },
  pause: { d: 'M5.6 4.4h2.6v11.2H5.6ZM11.8 4.4h2.6v11.2h-2.6Z', fill: true },
  user: { d: 'M10 9.6a3.3 3.3 0 1 0 0-6.6a3.3 3.3 0 1 0 0 6.6ZM3.8 17c.6-3.2 3.1-5.2 6.2-5.2s5.6 2 6.2 5.2' },
  logout: { d: 'M8.4 3.6H5a1.6 1.6 0 0 0-1.6 1.6v9.6A1.6 1.6 0 0 0 5 16.4h3.4M12.6 6.4L16.2 10l-3.6 3.6M16.2 10H8' },
  arrowUpRight: { d: 'M6.4 13.6l7.2-7.2M7.6 6.4h6v6' },
  arrowLeft: { d: 'M16 10H4.4M8.6 5.8L4.4 10l4.2 4.2' },
  info: { d: 'M2.8 10a7.2 7.2 0 1 0 14.4 0a7.2 7.2 0 1 0-14.4 0M10 9.2v4.6M10 6.4v.01' },
  alert: { d: 'M10 3.4l7.2 12.6H2.8ZM10 8.4v3.4M10 14.2v.01' },
  lock: { d: 'M5.6 8.8h8.8a1.4 1.4 0 0 1 1.4 1.4v5.4a1.4 1.4 0 0 1-1.4 1.4H5.6a1.4 1.4 0 0 1-1.4-1.4v-5.4a1.4 1.4 0 0 1 1.4-1.4ZM7 8.8V6.6a3 3 0 0 1 6 0v2.2' },
  edit: { d: 'M12.8 4l3.2 3.2-8.6 8.6-3.8.6.6-3.8ZM11.2 5.6l3.2 3.2' },
  trash: { d: 'M3.8 5.6h12.4M8 5.6V3.8h4v1.8M5.4 5.6l.8 10.6a1.4 1.4 0 0 0 1.4 1.3h4.8a1.4 1.4 0 0 0 1.4-1.3l.8-10.6' },
  send: { d: 'M3.6 10.2l12.8-6-5 13.2-2.2-5.4ZM9.2 12l7.2-7.8' },
  refresh: { d: 'M15.4 7.8A5.8 5.8 0 1 0 15.8 12M15.6 3.8v4h-4' },
  sliders: { d: 'M3.6 6h12.8M3.6 14h12.8M5.6 6a1.6 1.6 0 1 0 3.2 0a1.6 1.6 0 1 0-3.2 0M11.2 14a1.6 1.6 0 1 0 3.2 0a1.6 1.6 0 1 0-3.2 0' },
  sun: { d: 'M6.6 10a3.4 3.4 0 1 0 6.8 0a3.4 3.4 0 1 0-6.8 0M10 2.6v1.6M10 15.8v1.6M2.6 10h1.6M15.8 10h1.6M4.8 4.8l1.1 1.1M14.1 14.1l1.1 1.1M4.8 15.2l1.1-1.1M14.1 5.9l1.1-1.1' },
  moon: { d: 'M15.6 12.4A6.4 6.4 0 0 1 7.6 4.4a6.4 6.4 0 1 0 8 8Z' },
  card: { d: 'M3.2 5.6h13.6a1.4 1.4 0 0 1 1.4 1.4v6a1.4 1.4 0 0 1-1.4 1.4H3.2A1.4 1.4 0 0 1 1.8 13V7a1.4 1.4 0 0 1 1.4-1.4ZM1.8 8.6h16.4M4.8 11.6h3' },
  wallet: { d: 'M3.4 6.4h12.2a1.4 1.4 0 0 1 1.4 1.4v7.4a1.4 1.4 0 0 1-1.4 1.4H4.8a1.4 1.4 0 0 1-1.4-1.4ZM3.4 6.4V5.2a1.4 1.4 0 0 1 1.4-1.4h8.6M13.2 11.4h1.2' },
  volume: { d: 'M3.6 7.6h2.8l4-3.4v11.6l-4-3.4H3.6ZM13.4 7.4a3.8 3.8 0 0 1 0 5.2M15.6 5.2a6.8 6.8 0 0 1 0 9.6' },
  muted: { d: 'M3.6 7.6h2.8l4-3.4v11.6l-4-3.4H3.6ZM13.4 8l3.8 4M17.2 8l-3.8 4' },
  mic: { d:'M7.6 4.8a2.4 2.4 0 0 1 4.8 0v4.6a2.4 2.4 0 0 1-4.8 0ZM5 9.6a5 5 0 0 0 10 0M10 14.6v2.6' },
  download: { d: 'M10 3.6v9M6.4 9.2l3.6 3.6 3.6-3.6M4 16.4h12' },
  star: { d: 'M10 3.2l2 4.3 4.6.5-3.4 3.1 1 4.6-4.2-2.4-4.2 2.4 1-4.6-3.4-3.1 4.6-.5Z' },
} satisfies Record<string, IconDef>;

export type IconName = keyof typeof ICONS;

const SPARK_ON = '#d99a16';

export function Icon({
  name,
  size = 20,
  spark = false,
  className,
  label,
  strokeWidth = 1.6,
}: {
  name: IconName;
  size?: number;
  /** Light this icon's spark (the active navigation item). */
  spark?: boolean;
  className?: string;
  /** Accessible name. Omit for decorative icons next to visible text. */
  label?: string;
  strokeWidth?: number;
}) {
  const def: IconDef = ICONS[name];
  return (
    <svg
      viewBox="0 0 20 20"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={cn('shrink-0', className)}
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      focusable="false"
    >
      <path d={def.d} fill={def.fill ? 'currentColor' : 'none'} stroke={def.fill ? 'none' : undefined} />
      {def.spark && <circle cx={def.spark[0]} cy={def.spark[1]} r={def.spark[2]} fill={spark ? SPARK_ON : 'currentColor'} stroke="none" />}
    </svg>
  );
}
