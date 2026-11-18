/** Illustrative public integration contract, not an internal engine struct. */
export interface SignalSummary {
  symbol: string;
  asOf: string | null;
  action: 'WAIT' | 'LONG' | 'SHORT';
  state: 'available' | 'unavailable';
  modelVersion: string | null;
}

export const unavailableSignal: SignalSummary = {
  symbol: 'DEMO', asOf: null, action: 'WAIT', state: 'unavailable', modelVersion: null,
};

/** Reject extra fields instead of rendering or retaining private evaluation data. */
export function parseSignalSummary(value: unknown): SignalSummary {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid signal response');
  const v = value as Record<string, unknown>;
  const keys = ['symbol', 'asOf', 'action', 'state', 'modelVersion'];
  if (Object.keys(v).length !== keys.length || Object.keys(v).some(key => !keys.includes(key))) throw new Error('Invalid signal response');
  if (typeof v.symbol !== 'string' || !/^[A-Z0-9-]{1,20}$/.test(v.symbol)
    || !['WAIT', 'LONG', 'SHORT'].includes(String(v.action))
    || !['available', 'unavailable'].includes(String(v.state))
    || !(v.asOf === null || (typeof v.asOf === 'string' && Number.isFinite(Date.parse(v.asOf))))
    || !(v.modelVersion === null || (typeof v.modelVersion === 'string' && v.modelVersion.length <= 80))
    || (v.state === 'unavailable' && v.action !== 'WAIT')
    || (v.state === 'available' && (!v.asOf || !v.modelVersion))) throw new Error('Invalid signal response');
  return { symbol: v.symbol, asOf: v.asOf, action: v.action, state: v.state, modelVersion: v.modelVersion } as SignalSummary;
}
