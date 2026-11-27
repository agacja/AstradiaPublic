// Astrocartography calculation utilities
// Calculates MC, IC, ASC, DSC lines for all planets based on natal chart
import { Body, MakeTime, SunPosition, GeoMoon, GeoVector, Ecliptic } from 'astronomy-engine';

// Julian Day for a UTC instant (Meeus, ch. 7).
function getJulianDay(date: Date): number {
  const year = date.getUTCFullYear();
  const month = date.getUTCMonth() + 1;
  const day = date.getUTCDate();
  const hour = date.getUTCHours() + date.getUTCMinutes() / 60 + date.getUTCSeconds() / 3600;

  let y = year;
  let m = month;

  if (m <= 2) {
    y -= 1;
    m += 12;
  }

  const a = Math.floor(y / 100);
  const b = 2 - a + Math.floor(a / 4);

  return Math.floor(365.25 * (y + 4716)) + Math.floor(30.6001 * (m + 1)) + day + hour / 24 + b - 1524.5;
}

const DEG = Math.PI / 180;
const RAD = 180 / Math.PI;
const OBLIQUITY = 23.4393 * DEG; // Obliquity of ecliptic (J2000)
const HORIZON_STEP = 1; // degrees along an ASC/DSC curve between two of its points (about 111 km)
const HORIZON_LATITUDE = 75; // ASC/DSC curves are drawn between these latitudes, north and south

export interface AstroLine {
  planet: string;
  planetSymbol: string;
  lineType: 'MC' | 'IC' | 'ASC' | 'DSC';
  color: string;
  points: { latitude: number; longitude: number }[];
}

/** Where a planet stood on the sky: right ascension, declination and zodiac (ecliptic) longitude, in degrees. */
export interface SkyPlanet {
  name: string;
  ra: number;
  dec: number;
  lon: number;
}

/** The sky the lines are drawn from: the planets and Greenwich sidereal time (degrees). */
export interface Sky {
  gmst: number;
  planets: SkyPlanet[];
}

export interface AstroCartographyData {
  lines: AstroLine[];
  birthLocation: { latitude: number; longitude: number };
  sky: Sky;
}

interface PlanetEquatorial {
  name: string;
  symbol: string;
  color: string;
  ra: number;       // Right Ascension in degrees
  dec: number;      // Declination in degrees
  eclLon: number;   // Ecliptic longitude
}

const PLANET_INFO = [
  { name: 'Sun', symbol: '☉', color: '#FFD700', body: Body.Sun },
  { name: 'Moon', symbol: '☽', color: '#C0C0C0', body: Body.Moon },
  { name: 'Mercury', symbol: '☿', color: '#8FBC8F', body: Body.Mercury },
  { name: 'Venus', symbol: '♀︎', color: '#FF69B4', body: Body.Venus },
  { name: 'Mars', symbol: '♂︎', color: '#FF4500', body: Body.Mars },
  { name: 'Jupiter', symbol: '♃', color: '#9370DB', body: Body.Jupiter },
  { name: 'Saturn', symbol: '♄', color: '#DAA520', body: Body.Saturn },
  { name: 'Uranus', symbol: '♅', color: '#00CED1', body: Body.Uranus },
  { name: 'Neptune', symbol: '♆', color: '#4169E1', body: Body.Neptune },
  { name: 'Pluto', symbol: '♇', color: '#8B4513', body: Body.Pluto },
];

function normalizeAngle(a: number): number {
  a = a % 360;
  if (a < 0) a += 360;
  return a;
}

function normalizeLon(a: number): number {
  // Normalize to -180 to 180
  a = a % 360;
  if (a > 180) a -= 360;
  if (a < -180) a += 360;
  return a;
}

// Add the next point of a curve. Where the curve crosses the antimeridian
// (longitude ±180), it first gets a point on each side of it, on the straight
// piece between its two neighbours, so a map can cut the curve there without
// leaving a gap.
function addCurvePoint(points: AstroLine['points'], latitude: number, longitude: number): void {
  const previous = points[points.length - 1];
  if (previous && Math.abs(longitude - previous.longitude) > 180) {
    const edge = previous.longitude < 0 ? -180 : 180;
    const before = Math.abs(edge - previous.longitude);
    const after = Math.abs(-edge - longitude);
    const share = before + after > 0 ? before / (before + after) : 0;
    const crossingLat = previous.latitude + (latitude - previous.latitude) * share;
    points.push({ latitude: crossingLat, longitude: edge }, { latitude: crossingLat, longitude: -edge });
  }
  points.push({ latitude, longitude });
}

// Get equatorial coordinates (RA, Dec) from ecliptic longitude
// For planets with non-zero ecliptic latitude, this is an approximation
function eclipticToEquatorial(eclLon: number, eclLat: number = 0): { ra: number; dec: number } {
  const lambda = eclLon * DEG;
  const beta = eclLat * DEG;

  const sinDec = Math.sin(beta) * Math.cos(OBLIQUITY) +
                 Math.cos(beta) * Math.sin(OBLIQUITY) * Math.sin(lambda);
  const dec = Math.asin(Math.max(-1, Math.min(1, sinDec)));

  const y = Math.sin(lambda) * Math.cos(OBLIQUITY) - Math.tan(beta) * Math.sin(OBLIQUITY);
  const x = Math.cos(lambda);
  let ra = Math.atan2(y, x) * RAD;
  ra = normalizeAngle(ra);

  return { ra, dec: dec * RAD };
}

// Get planet positions at birth time with equatorial coords
function getPlanetPositions(birthDate: Date): PlanetEquatorial[] {
  const astroTime = MakeTime(birthDate);
  const planets: PlanetEquatorial[] = [];

  for (const info of PLANET_INFO) {
    let eclLon: number;
    let eclLat: number = 0;

    if (info.body === Body.Sun) {
      const sunPos = SunPosition(astroTime);
      eclLon = sunPos.elon;
      eclLat = sunPos.elat;
    } else if (info.body === Body.Moon) {
      const geoMoon = GeoMoon(astroTime);
      const moonEcl = Ecliptic(geoMoon);
      eclLon = moonEcl.elon;
      eclLat = moonEcl.elat;
    } else {
      const geoVec = GeoVector(info.body, astroTime, true);
      const ecl = Ecliptic(geoVec);
      eclLon = ecl.elon;
      eclLat = ecl.elat;
    }

    const eq = eclipticToEquatorial(eclLon, eclLat);
    planets.push({
      name: info.name,
      symbol: info.symbol,
      color: info.color,
      ra: eq.ra,
      dec: eq.dec,
      eclLon,
    });
  }

  return planets;
}

// Get GMST in degrees at a given Julian Day
function getGMST(jd: number): number {
  const T = (jd - 2451545.0) / 36525;
  let gmst = 280.46061837 +
             360.98564736629 * (jd - 2451545.0) +
             0.000387933 * T * T -
             T * T * T / 38710000;
  return normalizeAngle(gmst);
}

// Calculate all astrocartography lines
export function calculateAstrocartography(
  birthDate: Date,
  birthLat: number,
  birthLon: number
): AstroCartographyData {
  const jd = getJulianDay(birthDate);
  const gmst = getGMST(jd);
  const planets = getPlanetPositions(birthDate);
  const lines: AstroLine[] = [];

  for (const planet of planets) {
    // ── MC Line (vertical) ──
    // Planet culminates where LST = RA
    // LST = GMST + geoLon => geoLon = RA - GMST
    const mcLon = normalizeLon(planet.ra - gmst);
    const mcPoints: { latitude: number; longitude: number }[] = [];
    for (let lat = -80; lat <= 80; lat += 5) {
      mcPoints.push({ latitude: lat, longitude: mcLon });
    }
    lines.push({
      planet: planet.name,
      planetSymbol: planet.symbol,
      lineType: 'MC',
      color: planet.color,
      points: mcPoints,
    });

    // ── IC Line (vertical, 180° from MC) ──
    const icLon = normalizeLon(mcLon + 180);
    const icPoints: { latitude: number; longitude: number }[] = [];
    for (let lat = -80; lat <= 80; lat += 5) {
      icPoints.push({ latitude: lat, longitude: icLon });
    }
    lines.push({
      planet: planet.name,
      planetSymbol: planet.symbol,
      lineType: 'IC',
      color: planet.color,
      points: icPoints,
    });

    // ── ASC and DSC Lines (curves) ──
    // The planet is on the horizon along one great circle: every place a
    // quarter turn from the place it stands over (latitude = its declination,
    // on its MC line). Walk that circle in equal steps, by the bearing from
    // that place: from due south of it, through due east and west, to due
    // north. Each bearing gives one latitude, and the hour angle H at which
    // the planet sets there (cos H = -tan(lat) * tan(Dec)); it rises at -H.
    // Stepping longitude instead loses the lines of a planet near the
    // celestial equator, whose circle runs almost along the two meridians 90°
    // either side of its MC line; stepping latitude leaves a gap where the
    // two curves turn and meet, at latitude 90° minus the declination.
    const ascPoints: { latitude: number; longitude: number }[] = [];
    const dscPoints: { latitude: number; longitude: number }[] = [];
    const decRad = planet.dec * DEG;

    for (let bearing = 180; bearing >= 0; bearing -= HORIZON_STEP) {
      const bearingRad = bearing * DEG;
      const geoLat = Math.asin(Math.cos(bearingRad) * Math.cos(decRad)) * RAD;

      // Skip extreme latitudes
      if (Math.abs(geoLat) > HORIZON_LATITUDE) continue;

      const H = Math.atan2(Math.sin(bearingRad), -Math.cos(bearingRad) * Math.sin(decRad)) * RAD;
      // Rising (eastern horizon) = ASC, west of the MC line
      addCurvePoint(ascPoints, geoLat, normalizeLon(mcLon - H));
      // Setting (western horizon) = DSC, east of the MC line
      addCurvePoint(dscPoints, geoLat, normalizeLon(mcLon + H));
    }

    // The points run from south to north, the order they are drawn in.
    lines.push({
      planet: planet.name,
      planetSymbol: planet.symbol,
      lineType: 'ASC',
      color: planet.color,
      points: ascPoints,
    });

    lines.push({
      planet: planet.name,
      planetSymbol: planet.symbol,
      lineType: 'DSC',
      color: planet.color,
      points: dscPoints,
    });
  }

  return {
    lines,
    birthLocation: { latitude: birthLat, longitude: birthLon },
    sky: { gmst, planets: planets.map(({ name, ra, dec, eclLon }) => ({ name, ra, dec, lon: eclLon })) },
  };
}

// Get line type label
export function getLineLabel(lineType: string): string {
  switch (lineType) {
    case 'MC': return 'Midheaven';
    case 'IC': return 'Imum Coeli';
    case 'ASC': return 'Ascendant';
    case 'DSC': return 'Descendant';
    default: return lineType;
  }
}
