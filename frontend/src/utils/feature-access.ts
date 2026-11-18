/** Free access includes personal discovery, backtest research and social features. */
const FREE_TABS = new Set(['market', 'personal', 'kabbalah', 'backtest', 'dashboard', 'mycalendar']);

export function isFreeTab(tab: string): boolean {
  return FREE_TABS.has(tab);
}

export function canUseWidget(type: string, subscribed: boolean): boolean {
  return subscribed || ['personal-today', 'transits', 'kabbalah-basics'].includes(type);
}
