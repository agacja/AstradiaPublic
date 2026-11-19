import type { NatalChart } from './chart-types';

// localStorage keys for persisting chart data
export const STORAGE_KEY_BIRTH_DATA = 'astrocharts_birth_data';
export const STORAGE_KEY_NATAL_CHART = 'astrocharts_natal_chart';
const STORAGE_KEY_CHART_OWNER = 'astrocharts_birth_owner';

export function getChartOwner(): string | null {
  try { return localStorage.getItem(STORAGE_KEY_CHART_OWNER); }
  catch { return null; }
}

/** Legacy unowned caches must be restored from the authenticated account. */
export function syncChartOwner(userId: string | null): boolean {
  if (userId && getChartOwner() === userId) return false;
  try {
    clearSavedChartData();
    if (userId) localStorage.setItem(STORAGE_KEY_CHART_OWNER, userId);
    else localStorage.removeItem(STORAGE_KEY_CHART_OWNER);
  } catch { /* storage may be disabled */ }
  return true;
}

/** Birth data as stored in localStorage / submitted by BirthDataForm. */
export interface StoredBirthData {
  date: string;
  time: string;
  location: string;
  latitude: number;
  longitude: number;
  timezoneOffset: number;
  currentLocation: string;
  currentLatitude: number;
  currentLongitude: number;
}

export function loadSavedChartData(): { birthData: StoredBirthData | null; chart: NatalChart | null } {
  try {
    const savedBirthData = localStorage.getItem(STORAGE_KEY_BIRTH_DATA);
    const savedChart = localStorage.getItem(STORAGE_KEY_NATAL_CHART);
    if (savedBirthData && savedChart) {
      return { birthData: JSON.parse(savedBirthData), chart: JSON.parse(savedChart) };
    }
  } catch (error) {
    console.error('Error loading saved chart data:', error);
  }
  return { birthData: null, chart: null };
}

export function saveChartData(birthData: StoredBirthData, chart: NatalChart): void {
  try {
    localStorage.setItem(STORAGE_KEY_BIRTH_DATA, JSON.stringify(birthData));
    localStorage.setItem(STORAGE_KEY_NATAL_CHART, JSON.stringify(chart));
  } catch (error) {
    console.error('Error saving chart data:', error);
  }
}

export function clearSavedChartData(): void {
  localStorage.removeItem(STORAGE_KEY_BIRTH_DATA);
  localStorage.removeItem(STORAGE_KEY_NATAL_CHART);
}
