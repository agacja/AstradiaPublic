import { useEffect, useRef, useState, memo } from 'react';
import { reconnectDelay } from '../utils/reconnect-delay';
import { createChart, ColorType, CrosshairMode, CandlestickSeries, HistogramSeries, AreaSeries } from 'lightweight-charts';
import type { IChartApi, ISeriesApi, CandlestickData, Time, HistogramData } from 'lightweight-charts';
import { ChartLine } from 'lucide-react';
import { getCandles } from '../lib/hyperliquid-candles';
import '../styles/realtime-chart.css';

export type PriceChartVariant = 'area' | 'candles';

interface HyperliquidChartProps {
  symbol: string;
  height?: number;
  interval?: '1m' | '5m' | '15m' | '1h' | '4h' | '1d';
  /** Display name for the card head ("Live Hyperliquid price · 15m"). Defaults to SYMBOL/USD. */
  label?: string;
  /** 'area' = indigo line + gradient fill (astradia price card); 'candles' = OHLC candlesticks (trading screens). */
  variant?: PriceChartVariant;
  /** Render without the surrounding .ad-card chrome (the parent supplies the card). */
  bare?: boolean;
}

// Interval mapping for Hyperliquid API
type CandleInterval = '1m' | '3m' | '5m' | '15m' | '30m' | '1h' | '2h' | '4h' | '8h' | '12h' | '1d' | '3d' | '1w' | '1M';

/** One run of the chart effect for a given symbol + interval. */
type ChartSession = { live: boolean; failedAttempts: number };

const INTERVAL_TO_MS: Record<string, number> = {
  '1m': 60 * 1000,
  '5m': 5 * 60 * 1000,
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '4h': 4 * 60 * 60 * 1000,
  '1d': 24 * 60 * 60 * 1000,
};

const INTERVALS = ['1m', '5m', '15m', '1h', '4h', '1d'] as const;

// Custom theme colors matching the app - exported for use across all charts
// Light theme — the chart canvas is drawn, not styled by CSS, so its palette
// lives here. Ink-on-paper: light surface, dark text, saturated-but-readable
// marks (validated against a #eef3fb surface).
// eslint-disable-next-line react-refresh/only-export-components -- shared palette consumed by RealTimeChart/CommodityRealTimeChart
export const CHART_COLORS = {
  background: '#f4f7fc', // light blue paper
  gridLines: '#dbe4f1', // soft blue rules
  text: '#5b6485', // slate label ink
  crosshair: '#4f46e5', // indigo
  upColor: '#1b70da', // blue bullish (matches orderbook buy)
  downColor: '#7c3aed', // purple bearish (matches orderbook sell)
  volumeUp: 'rgba(27, 112, 218, 0.35)',
  volumeDown: 'rgba(124, 58, 237, 0.32)',
  // astradia "area" variant — drawn straight on the #f2f1fb card
  cardBackground: '#f2f1fb',
  gridSoft: 'rgba(124, 131, 242, 0.18)',
  textSoft: '#59599b',
  areaLine: '#5b63e8',
  areaTop: 'rgba(124, 131, 242, 0.35)',
  areaBottom: 'rgba(124, 131, 242, 0)',
};

function HyperliquidChartComponent({ symbol, height = 620, interval = '15m', label, variant = 'candles', bare = false }: HyperliquidChartProps) {
  const chartContainerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const candleSeriesRef = useRef<ISeriesApi<'Candlestick'> | ISeriesApi<'Area'> | null>(null);
  const volumeSeriesRef = useRef<ISeriesApi<'Histogram'> | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [currentPrice, setCurrentPrice] = useState<number | null>(null);
  const [priceChange, setPriceChange] = useState<number>(0);
  const [isConnected, setIsConnected] = useState(false);
  const [selectedInterval, setSelectedInterval] = useState(interval);
  const [isLoading, setIsLoading] = useState(true);

  const isArea = variant === 'area';

  // Push candle data into whichever price series is mounted (area = close line).
  const setPriceData = (candles: CandlestickData<Time>[]) => {
    const s = candleSeriesRef.current;
    if (!s) return;
    if (isArea) (s as ISeriesApi<'Area'>).setData(candles.map(c => ({ time: c.time, value: c.close })));
    else (s as ISeriesApi<'Candlestick'>).setData(candles);
  };
  const updatePrice = (candle: CandlestickData<Time>) => {
    const s = candleSeriesRef.current;
    if (!s) return;
    if (isArea) (s as ISeriesApi<'Area'>).update({ time: candle.time, value: candle.close });
    else (s as ISeriesApi<'Candlestick'>).update(candle);
  };

  useEffect(() => {
    if (!chartContainerRef.current) return;

    // Create chart with custom indigo/purple theme
    const chart = createChart(chartContainerRef.current, {
      layout: {
        background: { type: ColorType.Solid, color: isArea ? CHART_COLORS.cardBackground : CHART_COLORS.background },
        textColor: isArea ? CHART_COLORS.textSoft : CHART_COLORS.text,
        fontFamily: "'Geist Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
        fontSize: 10,
      },
      grid: {
        vertLines: { color: isArea ? 'transparent' : CHART_COLORS.gridLines },
        horzLines: { color: isArea ? CHART_COLORS.gridSoft : CHART_COLORS.gridLines },
      },
      crosshair: {
        mode: CrosshairMode.Normal,
        vertLine: {
          color: CHART_COLORS.crosshair,
          width: 1,
          style: 2,
        },
        horzLine: {
          color: CHART_COLORS.crosshair,
          width: 1,
          style: 2,
        },
      },
      rightPriceScale: {
        borderColor: isArea ? CHART_COLORS.gridSoft : CHART_COLORS.gridLines,
        scaleMargins: {
          top: 0.05,
          bottom: 0.25, // More space for volume
        },
      },
      timeScale: {
        borderColor: isArea ? CHART_COLORS.gridSoft : CHART_COLORS.gridLines,
        timeVisible: true,
        secondsVisible: selectedInterval === '1m',
      },
      width: chartContainerRef.current.clientWidth,
      height: height,
    });

    chartRef.current = chart;

    // Price series: astradia area line (indigo → transparent fill) or candlesticks
    if (isArea) {
      const areaSeries = chart.addSeries(AreaSeries, {
        lineColor: CHART_COLORS.areaLine,
        topColor: CHART_COLORS.areaTop,
        bottomColor: CHART_COLORS.areaBottom,
        lineWidth: 2,
        priceLineColor: CHART_COLORS.areaLine,
        crosshairMarkerBackgroundColor: CHART_COLORS.areaLine,
        crosshairMarkerBorderColor: CHART_COLORS.cardBackground,
      });
      candleSeriesRef.current = areaSeries as ISeriesApi<'Area'>;
    } else {
      const candleSeries = chart.addSeries(CandlestickSeries, {
        upColor: CHART_COLORS.upColor,
        downColor: CHART_COLORS.downColor,
        borderUpColor: CHART_COLORS.upColor,
        borderDownColor: CHART_COLORS.downColor,
        wickUpColor: CHART_COLORS.upColor,
        wickDownColor: CHART_COLORS.downColor,
      });
      candleSeriesRef.current = candleSeries as ISeriesApi<'Candlestick'>;
    }

    // Create volume series with purple theme
    const volumeSeries = chart.addSeries(HistogramSeries, {
      color: CHART_COLORS.volumeUp,
      priceFormat: {
        type: 'volume',
      },
      priceScaleId: '',
    });
    volumeSeries.priceScale().applyOptions({
      scaleMargins: {
        top: 0.75, // Volume takes up bottom 25% of chart
        bottom: 0,
      },
    });
    volumeSeriesRef.current = volumeSeries as ISeriesApi<'Histogram'>;

    // Everything this run starts (history request, socket, reconnect timer)
    // belongs to this symbol + interval. `live` goes false on cleanup, so a
    // slow response or a late timer from the previous selection can never
    // write into the chart that replaced it.
    const session: ChartSession = { live: true, failedAttempts: 0 };

    // Fetch initial historical data from Hyperliquid
    fetchHistoricalData(session);

    // Connect to WebSocket for real-time updates
    connectWebSocket(session);

    // Handle resize
    const handleResize = () => {
      if (chartContainerRef.current && chartRef.current) {
        chartRef.current.applyOptions({
          width: chartContainerRef.current.clientWidth,
        });
      }
    };

    window.addEventListener('resize', handleResize);

    return () => {
      session.live = false;
      window.removeEventListener('resize', handleResize);
      closeSocket();
      // Null the refs BEFORE remove() so late messages/frames see them gone
      candleSeriesRef.current = null;
      volumeSeriesRef.current = null;
      if (chartRef.current) {
        chartRef.current.remove();
        chartRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [symbol, selectedInterval, height, isArea]);

  // Cancel a pending reconnect and drop the socket without letting it call back.
  const closeSocket = () => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    const ws = wsRef.current;
    if (!ws) return;
    // Detach ALL handlers before closing: a no-arg close() yields close
    // code 1005, so the `code !== 1000` reconnect check passes and a
    // stale onclose resurrects the socket 3s after unmount — every tick
    // then calls update() on the disposed chart ("Object is disposed").
    ws.onopen = null;
    ws.onmessage = null;
    ws.onerror = null;
    ws.onclose = null;
    ws.close();
    wsRef.current = null;
  };

  const fetchHistoricalData = async (session: ChartSession) => {
    setIsLoading(true);
    try {
      // Calculate start time based on interval (get ~500 candles)
      const intervalMs = INTERVAL_TO_MS[selectedInterval] || INTERVAL_TO_MS['15m'];
      const startTime = Date.now() - (500 * intervalMs);

      const data = await getCandles(symbol, selectedInterval as CandleInterval, startTime);

      // The selection changed while this was in flight: the series refs now
      // point at the NEW chart, and these candles are for the old one.
      if (!session.live) return;
      if (!candleSeriesRef.current || !volumeSeriesRef.current || !data) return;

      // Transform Hyperliquid candle data to lightweight-charts format
      const candles: CandlestickData<Time>[] = data.map((d: { t: number; o: string; h: string; l: string; c: string }) => ({
        time: Math.floor(d.t / 1000) as Time,
        open: parseFloat(d.o),
        high: parseFloat(d.h),
        low: parseFloat(d.l),
        close: parseFloat(d.c),
      }));

      const volumes: HistogramData<Time>[] = data.map((d: { t: number; o: string; c: string; v: string }) => ({
        time: Math.floor(d.t / 1000) as Time,
        value: parseFloat(d.v),
        color: parseFloat(d.c) >= parseFloat(d.o) ? CHART_COLORS.volumeUp : CHART_COLORS.volumeDown,
      }));

      setPriceData(candles);
      volumeSeriesRef.current.setData(volumes);

      // Set initial price
      if (candles.length > 0) {
        const lastCandle = candles[candles.length - 1];
        setCurrentPrice(lastCandle.close);
        if (candles.length > 1) {
          const firstCandle = candles[0];
          setPriceChange(((lastCandle.close - firstCandle.open) / firstCandle.open) * 100);
        }
      }
    } catch (error) {
      if (session.live) console.error('Error fetching Hyperliquid candle data:', error);
    } finally {
      // A superseded request must not clear the new selection's loading state.
      if (session.live) setIsLoading(false);
    }
  };

  const connectWebSocket = (session: ChartSession) => {
    if (!session.live) return;
    // Close existing connection cleanly
    closeSocket();

    // Hyperliquid WebSocket for candle updates
    const wsUrl = 'wss://api.hyperliquid.xyz/ws';

    try {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        session.failedAttempts = 0;
        setIsConnected(true);
        // Subscribe to candle updates for this symbol
        ws.send(JSON.stringify({
          method: 'subscribe',
          subscription: {
            type: 'candle',
            coin: symbol,
            interval: selectedInterval,
          },
        }));
      };

      ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);

          // Handle candle updates
          if (message.channel === 'candle' && message.data) {
            const d = message.data;
            const candle: CandlestickData<Time> = {
              time: Math.floor(d.t / 1000) as Time,
              open: parseFloat(d.o),
              high: parseFloat(d.h),
              low: parseFloat(d.l),
              close: parseFloat(d.c),
            };

            updatePrice(candle);

            if (volumeSeriesRef.current) {
              volumeSeriesRef.current.update({
                time: Math.floor(d.t / 1000) as Time,
                value: parseFloat(d.v),
                color: parseFloat(d.c) >= parseFloat(d.o) ? CHART_COLORS.volumeUp : CHART_COLORS.volumeDown,
              });
            }

            setCurrentPrice(parseFloat(d.c));
          }
        } catch {
          // Silently ignore parse errors
        }
      };

      ws.onerror = () => {
        setIsConnected(false);
      };

      ws.onclose = (event) => {
        setIsConnected(false);
        // Reconnect if it wasn't a clean close. The timer is tracked so
        // cleanup can cancel it, and it re-checks the session so it can only
        // ever reconnect the selection it was scheduled for.
        if (event.code !== 1000 && session.live) {
          const wait = reconnectDelay(session.failedAttempts);
          session.failedAttempts += 1;
          reconnectTimerRef.current = setTimeout(() => {
            reconnectTimerRef.current = null;
            connectWebSocket(session);
          }, wait);
        }
      };
    } catch (e) {
      console.error('Failed to create WebSocket:', e);
      setIsConnected(false);
    }
  };

  // The chart effect owns the socket: changing the interval re-runs it, and its
  // cleanup closes the old connection with handlers detached. Closing here too
  // fired the old onclose and scheduled a reconnect for the previous interval.
  const handleIntervalChange = (newInterval: string) => {
    setSelectedInterval(newInterval as typeof interval);
  };

  const formatPrice = (price: number) => {
    if (price >= 1000) return price.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    if (price >= 1) return price.toFixed(4);
    return price.toFixed(6);
  };

  const status = isLoading ? 'loading' : isConnected ? 'live' : 'connecting';

  return (
    <div className={`${bare ? '' : 'ad-card '}rtc-card`}>
      {/* Head: label + live price */}
      <div className="rtc-head">
        <div className="rtc-head-label">
          <ChartLine size={14} />
          Live {label ?? `${symbol}/USD`} price · {selectedInterval}
        </div>
        {currentPrice !== null && (
          <div className="rtc-price-wrap">
            <span className="rtc-price">${formatPrice(currentPrice)}</span>
            <span className={`rtc-change ${priceChange >= 0 ? 'up' : 'down'}`}>
              {priceChange >= 0 ? '+' : ''}{priceChange.toFixed(2)}%
            </span>
          </div>
        )}
      </div>

      {/* Chart container */}
      <div ref={chartContainerRef} className="rtc-canvas" />

      {/* Interval pills + refresh note */}
      <div className="rtc-foot">
        <div className="rtc-intervals" role="group" aria-label="Chart interval">
          {INTERVALS.map((int) => (
            <button
              key={int}
              type="button"
              className={`rtc-pill${selectedInterval === int ? ' active' : ''}`}
              onClick={() => handleIntervalChange(int)}
            >
              {int}
            </button>
          ))}
        </div>
        <span className="rtc-note">
          <span className={`rtc-dot${isConnected && !isLoading ? '' : ' off'}`} />
          {status} · streaming updates
        </span>
      </div>
    </div>
  );
}

export const HyperliquidChart = memo(HyperliquidChartComponent);
