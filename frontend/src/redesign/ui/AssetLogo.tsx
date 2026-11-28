import type { SVGProps } from 'react';
import { cn } from './cn';
import { assetLogoKey, hasAssetLogo, MARKS } from './assetMarks';

/**
 * Real brand marks for the crypto assets the app shows, one quiet treatment:
 * a bare monochrome mark in the surrounding text colour, never on a tile.
 *
 * Source: @web3icons/core 4.0.61 (MIT), "mono" token marks; the HYPE mark is
 * that package's Hyperliquid (HyperEVM) network mark. Vendored so only these
 * few ship. Trademarks belong to their owners. Commodities (gold, silver, oil)
 * have no brand mark, so `AssetLogo` renders nothing for them; use
 * `hasAssetLogo` to keep a slot when a list mixes both.
 */
export function AssetLogo({
  symbol,
  size = 20,
  title,
  className,
  ...rest
}: { symbol: string | null | undefined; size?: number | string; title?: string } & Omit<SVGProps<SVGSVGElement>, 'children'>) {
  const key = assetLogoKey(symbol);
  if (!key) return null;
  const mark = MARKS[key];
  if (!mark) return null;
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="currentColor"
      role={title ? 'img' : undefined}
      aria-label={title}
      aria-hidden={title ? undefined : true}
      className={cn('shrink-0', className)}
      {...rest}
    >
      {mark.paths.map((p, i) => (
        <path key={i} d={p.d} fillRule={p.rule} clipRule={p.rule} />
      ))}
    </svg>
  );
}

/**
 * A large, faded mark for an asset's own page, behind its header. By default
 * it is as tall as its parent, so it never runs under the sticky top bar or
 * past the header. Put it first inside a `relative isolate` parent and give
 * the parent's other children `relative` so text stays above it.
 */
export function AssetWatermark({ symbol, className }: { symbol: string | null | undefined; className?: string }) {
  if (!hasAssetLogo(symbol)) return null;
  return (
    <AssetLogo
      symbol={symbol}
      size="100%"
      className={cn('pointer-events-none absolute inset-y-0 right-0 aspect-square h-full w-auto max-w-[44vw] select-none text-violet opacity-[0.07]', className)}
    />
  );
}
