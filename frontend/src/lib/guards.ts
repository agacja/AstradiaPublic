// Narrowing for values that come from outside the program.

/** A plain object: not null, and not a list. */
export const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

/**
 * A list whose elements are still unknown. `Array.isArray` alone narrows to
 * `any[]`, which would let an unchecked element through as any type.
 */
export const isList = (value: unknown): value is unknown[] => Array.isArray(value);
