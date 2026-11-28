import { useEffect, useLayoutEffect, useRef, useState, memo } from 'react';
import { reconnectDelay } from '../utils/reconnect-delay';
import { createChart, ColorType, CrosshairMode, CandlestickSeries, HistogramSeries, AreaSeries, LineStyle } from 'lightweight-charts';
import type { IChartApi, ISeriesApi, CandlestickData, Time, HistogramData } from 'lightweight-charts';
import { getCandles } from '../lib/hyperliquid-candles';
import { readLiveCandle } from '../utils/candle-message';
import { AssetLogo, cn, Delta, Segmented, Skeleton, StateLabel, type SegmentOption } from '../redesign/ui';

export type PriceChartVariant = 'area' | 'candles';

interface HyperliquidChartProps {
  symbol: string;
  height?: number;
  interval?: '1m' | '5m' | '15m' | '1h' | '4h' | '1d';
  /** Display name for the head ("Hyperliquid"). Defaults to SYMBOL/USD. */
  label?: string;
  /** 'area' = a line over a flat tint (the price card); 'candles' = OHLC candlesticks (trading screens). */
  variant?: PriceChartVariant;
  /** Render without the surrounding card (the parent supplies the surface). */
  bare?: boolean;
  /** Decimals of the head price. Left out, the precision follows the size of the price. */
  decimals?: number;
  /** What the foot names as the origin of the stream. Defaults to "Hyperliquid". */
  source?: string;
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
type Interval = (typeof INTERVALS)[number];
const INTERVAL_OPTIONS: SegmentOption<Interval>[] = INTERVALS.map((value) => ({ value, label: value }));
const VIEW_OPTIONS: SegmentOption<PriceChartVariant>[] = [
  { value: 'area', label: 'Line' },
  { value: 'candles', label: 'Candles' },
];

/**
 * The price charts' palette. The canvas is drawn, not styled by CSS, so its
 * colours live here. The original keys are the vellum (light) surface; `night`
 * holds the values for a chart that sits on a night panel. Up candles are
 * hollow gold, down candles solid rose, the line is violet (light) or pale
 * lavender (night) over a flat tint. No gradients.
 */
// eslint-disable-next-line react-refresh/only-export-components -- a shared palette, exported for other charts
export const CHART_COLORS = {
  background: 'transparent',
  gridLines: '#ece8f8',
  text: '#6a6490',
  crosshair: '#5646d1',
  crosshairLabel: '#5646d1',
  upColor: '#b8800d',
  downColor: '#c8455a',
  volumeUp: 'rgba(184, 128, 13, 0.22)',
  volumeDown: 'rgba(200, 69, 90, 0.2)',
  cardBackground: 'transparent',
  gridSoft: '#ece8f8',
  textSoft: '#6a6490',
  areaLine: '#5646d1',
  areaTop: 'rgba(86, 70, 209, 0.08)',
  areaBottom: 'rgba(86, 70, 209, 0.08)',
  fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif",
  night: {
    gridLines: '#3f3573',
    text: '#a99fd0',
    crosshair: '#a99fd0',
    crosshairLabel: '#31285a',
    upColor: '#f2c14e',
    downColor: '#ff9aaa',
    volumeUp: 'rgba(242, 193, 78, 0.2)',
    volumeDown: 'rgba(255, 154, 170, 0.2)',
    areaLine: '#d3cbec',
    areaTop: 'rgba(211, 203, 236, 0.07)',
    areaBottom: 'rgba(211, 203, 236, 0.07)',
  },
};

type Tone = 'light' | 'night';
const paletteFor = (tone: Tone) => (tone === 'night' ? { ...CHART_COLORS, ...CHART_COLORS.night } : CHART_COLORS);

function HyperliquidChartComponent({ symbol, height = 620, interval = '15m', label, variant = 'candles', bare = false, decimals, source }: HyperliquidChartProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const chartContainerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const candleSeriesRef = useRef<ISeriesApi<'Candlestick'> | ISeriesApi<'Area'> | null>(null);
  const volumeSeriesRef = useRef<ISeriesApi<'Histogram'> | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // The palette of the surface the chart sits on (a night panel or a vellum card).
  const paletteRef = useRef(paletteFor('light'));

  const [currentPrice, setCurrentPrice] = useState<number | null>(null);
  const [priceChange, setPriceChange] = useState<number>(0);
  const [isConnected, setIsConnected] = useState(false);
  const [selectedInterval, setSelectedInterval] = useState(interval);
  const [isLoading, setIsLoading] = useState(true);
  const [tone, setTone] = useState<Tone>('light');

  // A page chart (bare) lets the member switch between the line and candles;
  // the prop sets the view it opens with.
  const [shownVariant, setShownVariant] = useState<PriceChartVariant>(variant);
  useEffect(() => setShownVariant(variant), [variant]);
  const isArea = shownVariant === 'area';

  // A chart placed on a night panel takes the night palette; nothing to pass.
  useLayoutEffect(() => {
    setTone(rootRef.current?.closest('.rd-night') ? 'night' : 'light');
  }, []);

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

    const palette = paletteFor(chartContainerRef.current.closest('.rd-night') ? 'night' : 'light');
    paletteRef.current = palette;

    const chart = createChart(chartContainerRef.current, {
      autoSize: true,
      layout: {
        background: { type: ColorType.Solid, color: palette.background },
        textColor: palette.text,
        fontFamily: palette.fontFamily,
        fontSize: 12,
      },
      grid: {
        vertLines: { visible: false },
        horzLines: { color: palette.gridLines },
      },
      crosshair: {
        mode: CrosshairMode.Normal,
        vertLine: { color: palette.crosshair, width: 1, style: LineStyle.Dashed, labelBackgroundColor: palette.crosshairLabel },
        horzLine: { color: palette.crosshair, width: 1, style: LineStyle.Dashed, labelBackgroundColor: palette.crosshairLabel },
      },
      rightPriceScale: {
        borderVisible: false,
        scaleMargins: {
          top: 0.08,
          bottom: 0.25, // More space for volume
        },
      },
      timeScale: {
        borderVisible: false,
        timeVisible: true,
        secondsVisible: selectedInterval === '1m',
      },
      // A bare chart sits inside a page's panel: the page keeps the mouse wheel
      // and vertical swipes, so scrolling never gets caught on the chart. Drag
      // and pinch still pan and zoom it; a framed chart (the trading desk) keeps the wheel.
      handleScroll: bare ? { mouseWheel: false, pressedMouseMove: true, horzTouchDrag: true, vertTouchDrag: false } : true,
      handleScale: bare ? { mouseWheel: false, pinch: true, axisPressedMouseMove: true, axisDoubleClickReset: true } : true,
      width: chartContainerRef.current.clientWidth,
      height: height,
    });

    chartRef.current = chart;

    // Price series: a line over a flat tint, or candlesticks (hollow gold up, solid rose down)
    if (isArea) {
      const areaSeries = chart.addSeries(AreaSeries, {
        lineColor: palette.areaLine,
        topColor: palette.areaTop,
        bottomColor: palette.areaBottom,
        lineWidth: 2,
        priceLineColor: palette.areaLine,
        crosshairMarkerBackgroundColor: palette.areaLine,
        crosshairMarkerBorderColor: palette.areaLine,
      });
      candleSeriesRef.current = areaSeries as ISeriesApi<'Area'>;
    } else {
      const candleSeries = chart.addSeries(CandlestickSeries, {
        upColor: 'rgba(0, 0, 0, 0)',
        downColor: palette.downColor,
        borderUpColor: palette.upColor,
        borderDownColor: palette.downColor,
        wickUpColor: palette.upColor,
        wickDownColor: palette.downColor,
      });
      candleSeriesRef.current = candleSeries as ISeriesApi<'Candlestick'>;
    }

    const volumeSeries = chart.addSeries(HistogramSeries, {
      color: palette.volumeUp,
      priceFormat: {
        type: 'volume',
      },
      priceScaleId: '',
      // Volume is context under the price: no label of its own on the price scale.
      lastValueVisible: false,
      priceLineVisible: false,
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

    // The chart follows its container's size itself (autoSize).
    return () => {
      session.live = false;
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
  }, [symbol, selectedInterval, height, isArea, bare]);

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
      const intervalMs = INTERVAL_TO_MS[selectedInterval] ?? 15 * 60 * 1000;
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

      const palette = paletteRef.current;
      const volumes: HistogramData<Time>[] = data.map((d: { t: number; o: string; c: string; v: string }) => ({
        time: Math.floor(d.t / 1000) as Time,
        value: parseFloat(d.v),
        color: parseFloat(d.c) >= parseFloat(d.o) ? palette.volumeUp : palette.volumeDown,
      }));

      setPriceData(candles);
      volumeSeriesRef.current.setData(volumes);

      // Set initial price
      const firstCandle = candles[0];
      const lastCandle = candles[candles.length - 1];
      if (firstCandle && lastCandle) {
        setCurrentPrice(lastCandle.close);
        if (candles.length > 1) {
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
          // Anything but a whole candle is not for the chart.
          const live = readLiveCandle(JSON.parse(event.data));
          if (!live) return;
          const { volume, ...prices } = live;
          const candle: CandlestickData<Time> = { ...prices, time: live.time as Time };

          updatePrice(candle);
          volumeSeriesRef.current?.update({
            time: candle.time,
            value: volume,
            color: candle.close >= candle.open ? paletteRef.current.volumeUp : paletteRef.current.volumeDown,
          });
          setCurrentPrice(candle.close);
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
    if (decimals !== undefined) return price.toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
    if (price >= 1000) return price.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    if (price >= 1) return price.toFixed(4);
    return price.toFixed(6);
  };

  const night = tone === 'night';
  const streaming = isConnected && !isLoading;

  return (
    <div
      ref={rootRef}
      className={cn('flex min-w-0 flex-col gap-4 md:gap-5', !bare && 'rounded-[24px] border border-line bg-vellum p-5 md:p-7')}
    >
      {/* Head: what is charted, then the live price */}
      <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
        <p className={cn('flex min-w-0 items-center gap-2 text-[16px] font-semibold', night ? 'text-night-ink' : 'text-ink')}>
          <AssetLogo symbol={symbol} size={18} className={night ? 'text-night-ink' : 'text-ink-2'} />
          <span className="truncate">{label ?? `${symbol}/USD`}</span>
        </p>
        {currentPrice !== null ? (
          <p className="flex items-baseline gap-3">
            <span className={cn('font-display text-[30px] leading-none tabular-nums md:text-[38px]', night ? 'text-night-ink' : 'text-ink')}>${formatPrice(currentPrice)}</span>
            <Delta value={Math.round(priceChange * 100) / 100} tone={tone} className="text-[15px]" />
          </p>
        ) : (
          <Skeleton tone={tone} className="h-[30px] w-44 md:h-[38px]" />
        )}
      </div>

      {/* The canvas, with a quiet block in its place until the history arrives */}
      <div className="relative">
        <div ref={chartContainerRef} className="w-full" style={{ height }} />
        {isLoading && currentPrice === null && <Skeleton tone={tone} className="pointer-events-none absolute inset-0 rounded-[16px]" />}
      </div>

      {/* Interval choice + the stream's state */}
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <Segmented<Interval>
            size="sm"
            tone={tone}
            label="Chart interval"
            options={INTERVAL_OPTIONS}
            value={selectedInterval as Interval}
            onChange={handleIntervalChange}
          />
          {bare && <Segmented<PriceChartVariant> size="sm" tone={tone} label="Chart view" options={VIEW_OPTIONS} value={shownVariant} onChange={setShownVariant} />}
        </div>
        <StateLabel kind={streaming ? 'live' : 'rest'} tone={tone} className="text-[14px]">
          {streaming ? 'Live' : 'Connecting'} · {source ?? 'Hyperliquid'}
        </StateLabel>
      </div>
    </div>
  );
}

export const HyperliquidChart = memo(HyperliquidChartComponent);
