// Public market data only: Hyperliquid's unauthenticated candle snapshot.
// The product reads candles through an exchange SDK client that also carries
// account and order functions; this file is the read-only equivalent.
export interface Candle {
  t: number;
  o: string;
  h: string;
  l: string;
  c: string;
  v: string;
}

export async function getCandles(coin: string, interval: string, startTime: number, endTime?: number): Promise<Candle[]> {
  const response = await fetch('https://api.hyperliquid.xyz/info', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ type: 'candleSnapshot', req: { coin, interval, startTime, endTime: endTime ?? Date.now() } }),
  });
  if (!response.ok) throw new Error(`Candle request failed: ${response.status}`);
  return response.json();
}
