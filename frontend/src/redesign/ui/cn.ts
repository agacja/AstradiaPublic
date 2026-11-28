import { twMerge } from 'tailwind-merge';

/**
 * Joins class names, skipping falsy parts. Conflicting utilities resolve to
 * the last one given (tailwind-merge), so a screen can override a primitive's
 * colour or size with `className` and get exactly what it asked for.
 */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return twMerge(parts.filter(Boolean).join(' '));
}
