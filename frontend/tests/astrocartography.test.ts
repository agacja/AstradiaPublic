import assert from 'node:assert/strict';
import test from 'node:test';
import { calculateAstrocartography, getLineLabel } from '../src/utils/astrocartography.ts';
import type { AstroCartographyData, AstroLine, SkyPlanet } from '../src/utils/astrocartography.ts';

const DEG = Math.PI / 180;
const PLANETS = ['Sun', 'Moon', 'Mercury', 'Venus', 'Mars', 'Jupiter', 'Saturn', 'Uranus', 'Neptune', 'Pluto'];
const HORIZON_TYPES = ['ASC', 'DSC'] as const;
/** The rising and setting curves are drawn between these latitudes, in steps of one degree along the curve. */
const BAND = 75;
const STEP = 1;

const chart = calculateAstrocartography(new Date('2000-01-01T12:00:00Z'), 51.5074, -0.1278);
const lineOf = (data: AstroCartographyData, planet: string, type: AstroLine['lineType']) =>
  data.lines.find(l => l.planet === planet && l.lineType === type);
const byPlanet = (planet: string, type: AstroLine['lineType']) => lineOf(chart, planet, type);
const wrap = (lon: number) => ((((lon + 180) % 360) + 360) % 360) - 180;
/** The longitude a meridian line stands at: every point of an MC or IC line has it. */
function meridian(line: AstroLine): number {
  const longitude = line.points[0]?.longitude;
  assert.ok(longitude !== undefined, `${line.planet} ${line.lineType} has no points`);
  assert.ok(line.points.every(p => p.longitude === longitude), `${line.planet} ${line.lineType} is a meridian`);
  return longitude;
}

// Every tenth day of thirty years: 1,096 skies, in which a planet stands within
// half a degree of the celestial equator more than a hundred times.
const DAY = 86_400_000;
const skies: AstroCartographyData[] = [];
for (let time = Date.UTC(2000, 0, 1); time < Date.UTC(2030, 0, 1); time += 10 * DAY) {
  skies.push(calculateAstrocartography(new Date(time), 0, 0));
}

/** The planet's hour angle at a longitude, in degrees: 0 on its MC line, negative where it is still rising. */
const hourAngle = (data: AstroCartographyData, planet: SkyPlanet, longitude: number) => wrap(data.sky.gmst + longitude - planet.ra);
/** The planet's height above the horizon at a place, in degrees. */
function altitude(data: AstroCartographyData, planet: SkyPlanet, latitude: number, longitude: number): number {
  const hour = hourAngle(data, planet, longitude) * DEG;
  const sine = Math.sin(latitude * DEG) * Math.sin(planet.dec * DEG) + Math.cos(latitude * DEG) * Math.cos(planet.dec * DEG) * Math.cos(hour);
  return Math.asin(Math.max(-1, Math.min(1, sine))) / DEG;
}
/** The angle between two places as seen from the Earth's centre, in degrees. */
function apart(a: AstroLine['points'][number], b: AstroLine['points'][number]): number {
  const dLat = (b.latitude - a.latitude) * DEG;
  const dLon = (b.longitude - a.longitude) * DEG;
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(a.latitude * DEG) * Math.cos(b.latitude * DEG) * Math.sin(dLon / 2) ** 2;
  return 2 * Math.asin(Math.min(1, Math.sqrt(h))) / DEG;
}
/** The two points a curve gets where it crosses the antimeridian, one on each side of it. */
const onAntimeridian = (point: AstroLine['points'][number]) => Math.abs(point.longitude) === 180;
/** Each rising and setting line of each sky, with the planet it belongs to. */
function* horizonLines(): Generator<{ data: AstroCartographyData; planet: SkyPlanet; line: AstroLine; name: string }> {
  for (const data of skies) {
    for (const planet of data.sky.planets) {
      for (const type of HORIZON_TYPES) {
        const line = lineOf(data, planet.name, type);
        assert.ok(line, `${planet.name} ${type} is missing (declination ${planet.dec})`);
        yield { data, planet, line, name: `${planet.name} ${type} (declination ${planet.dec.toFixed(4)})` };
      }
    }
  }
}

test('every body has a meridian pair exactly half a turn apart', () => {
  for (const planet of PLANETS) {
    const mc = byPlanet(planet, 'MC');
    const ic = byPlanet(planet, 'IC');
    assert.ok(mc && ic, planet);
    assert.ok(Math.abs(wrap(meridian(ic) - meridian(mc) - 180)) < 1e-9, `${planet} IC opposes MC`);
  }
});

test('the Sun culminates near Greenwich at Greenwich noon', () => {
  // 2000-01-01 12:00 UTC: the equation of time is about -3 minutes, under one degree.
  const mc = byPlanet('Sun', 'MC');
  assert.ok(mc);
  assert.ok(Math.abs(meridian(mc)) < 1.5, `Sun MC at ${meridian(mc)}`);
});

test('every planet has all four of its lines, also when it stands near the celestial equator', () => {
  let nearEquator = 0;
  for (const data of skies) {
    assert.deepEqual(
      data.lines.map(line => `${line.planet} ${line.lineType}`),
      PLANETS.flatMap(planet => ['MC', 'IC', 'ASC', 'DSC'].map(type => `${planet} ${type}`)),
    );
    nearEquator += data.sky.planets.filter(planet => Math.abs(planet.dec) < 0.5).length;
  }
  // A planet this close to the celestial equator rises and sets along two lines that are almost
  // meridians. Curves stepped by longitude lose such lines, so the sweep must keep including these planets.
  assert.ok(nearEquator > 100, `${nearEquator} planets within 0.5 degrees of the celestial equator`);
  // Mars 0.05 degrees north of the celestial equator.
  const mars = calculateAstrocartography(new Date('1950-01-13T00:00:00Z'), 0, 0);
  const declination = mars.sky.planets.find(planet => planet.name === 'Mars')?.dec;
  assert.ok(declination !== undefined && Math.abs(declination) < 0.1, `Mars declination ${declination}`);
  for (const type of HORIZON_TYPES) {
    assert.ok((lineOf(mars, 'Mars', type)?.points.length ?? 0) >= 150, `Mars ${type}`);
  }
});

test('at every point of a rising or setting line the planet is on the horizon, rising on ASC and setting on DSC', () => {
  let points = 0;
  for (const { data, planet, line, name } of horizonLines()) {
    for (const point of line.points) {
      const height = Math.abs(altitude(data, planet, point.latitude, point.longitude));
      // A point at the antimeridian lies on the straight piece between two points of the curve, a little off it.
      assert.ok(height < (onAntimeridian(point) ? 0.02 : 1e-9), `${name}: altitude ${height} at ${point.latitude}, ${point.longitude}`);
      // Rising places are west of the planet's MC line (hour angle below 0), setting places east of it.
      const side = Math.sin(hourAngle(data, planet, point.longitude) * DEG) * (line.lineType === 'ASC' ? 1 : -1);
      assert.ok(side < 1e-9, `${name}: wrong side of the MC line at ${point.latitude}, ${point.longitude}`);
      points++;
    }
  }
  assert.ok(points > 3_000_000, `${points} points`);
});

test('a line runs from south to north without a gap, as far as the planet rises and sets or the map draws it', () => {
  for (const { data, planet, line, name } of horizonLines()) {
    const { points } = line;
    const first = points[0];
    const last = points[points.length - 1];
    assert.ok(first && last, name);
    points.forEach((point, i) => {
      assert.ok(Math.abs(point.latitude) <= BAND && Math.abs(point.longitude) <= 180, `${name}: off the map at ${point.latitude}, ${point.longitude}`);
      const previous = points[i - 1];
      if (!previous) return;
      assert.ok(point.latitude >= previous.latitude, `${name}: not south to north at ${point.latitude}`);
      // Neighbours are one step apart at most, the short way round the world.
      assert.ok(apart(previous, point) < STEP * 1.001, `${name}: a gap before ${point.latitude}, ${point.longitude}`);
      // A jump in longitude is a crossing of the antimeridian, with a point on each side of it.
      if (Math.abs(point.longitude - previous.longitude) > 4) {
        assert.ok(
          onAntimeridian(previous) && point.longitude === -previous.longitude && point.latitude === previous.latitude,
          `${name}: a jump before ${point.latitude}, ${point.longitude}`,
        );
      }
    });
    // North of 90 degrees less the size of its declination a planet never sets or never rises: there its two lines meet and end.
    const reach = 90 - Math.abs(planet.dec);
    if (reach <= BAND) {
      const other = lineOf(data, planet.name, line.lineType === 'ASC' ? 'DSC' : 'ASC');
      const otherFirst = other?.points[0];
      const otherLast = other?.points[other.points.length - 1];
      assert.ok(otherFirst && otherLast, name);
      assert.ok(Math.abs(first.latitude + reach) < 1e-6 && Math.abs(last.latitude - reach) < 1e-6, `${name}: ${first.latitude} to ${last.latitude}`);
      assert.ok(apart(first, otherFirst) < 1e-6, `${name}: open in the south`);
      assert.ok(apart(last, otherLast) < 1e-6, `${name}: open in the north`);
    } else {
      // A curve the map cuts off starts and ends within one step of the band's edges.
      assert.ok(first.latitude < STEP - BAND + 1e-9 && last.latitude > BAND - STEP - 1e-9, `${name}: ${first.latitude} to ${last.latitude}`);
    }
  }
});

test('a planet on the celestial equator rises and sets along the two meridians a quarter turn either side of its MC line', () => {
  // The March equinox of 2024, found as the moment the Sun's declination passes zero.
  const sunAt = (time: number) => calculateAstrocartography(new Date(time), 0, 0);
  const declination = (data: AstroCartographyData) => data.sky.planets.find(planet => planet.name === 'Sun')?.dec ?? NaN;
  let before = Date.UTC(2024, 2, 19);
  let after = Date.UTC(2024, 2, 21);
  assert.ok(declination(sunAt(before)) < 0 && declination(sunAt(after)) > 0);
  while (after - before > 1) {
    const middle = Math.floor((before + after) / 2);
    if (declination(sunAt(middle)) < 0) before = middle;
    else after = middle;
  }
  const data = sunAt(after);
  const sun = data.sky.planets.find(planet => planet.name === 'Sun');
  assert.ok(sun && Math.abs(sun.dec) < 1e-6, `declination ${sun?.dec}`);
  const mc = wrap(sun.ra - data.sky.gmst);
  for (const [type, offset] of [['ASC', -90], ['DSC', 90]] as const) {
    const points = lineOf(data, 'Sun', type)?.points ?? [];
    // The whole band the map draws, south to north, on one meridian: not the equator.
    assert.ok(points.length >= 149, `Sun ${type}: ${points.length} points`);
    assert.ok((points[0]?.latitude ?? 0) < STEP - BAND + 1e-9 && (points[points.length - 1]?.latitude ?? 0) > BAND - STEP - 1e-9, `Sun ${type}`);
    for (const point of points) {
      assert.ok(Math.abs(wrap(point.longitude - mc - offset)) < 1e-4, `Sun ${type}: ${point.longitude} at ${point.latitude}, MC line at ${mc}`);
    }
  }
});

test('the sky the lines were drawn from comes back with them', () => {
  const { gmst, planets } = chart.sky;
  // 2000-01-01 12:00 UTC is Julian day 2451545.0, where the formula's constant term is all that is left.
  assert.ok(Math.abs(gmst - 280.46061837) < 1e-9, `sidereal time ${gmst}`);
  assert.deepEqual(planets.map(p => p.name), PLANETS);
  for (const planet of planets) {
    assert.ok(planet.ra >= 0 && planet.ra < 360, `${planet.name} right ascension`);
    assert.ok(Math.abs(planet.dec) <= 90, `${planet.name} declination`);
    assert.ok(planet.lon >= 0 && planet.lon < 360, `${planet.name} longitude`);
    // A meridian line stands at the longitude where local sidereal time equals the planet's right
    // ascension: right ascension less Greenwich's sidereal time.
    const mc = byPlanet(planet.name, 'MC');
    assert.ok(mc, planet.name);
    assert.ok(Math.abs(wrap(meridian(mc) - (planet.ra - gmst))) < 1e-9, `${planet.name} MC`);
  }
  // Ten days after the December solstice the Sun is ten degrees into Capricorn and 23 degrees south.
  const sun = planets.find(p => p.name === 'Sun');
  assert.ok(sun);
  assert.ok(Math.abs(sun.lon - 280.4) < 0.5, `Sun longitude ${sun.lon}`);
  assert.ok(Math.abs(sun.dec + 23.0) < 0.2, `Sun declination ${sun.dec}`);
});

test('the birth location is echoed and labels are stable', () => {
  assert.deepEqual(chart.birthLocation, { latitude: 51.5074, longitude: -0.1278 });
  assert.equal(getLineLabel('MC'), 'Midheaven');
  assert.equal(getLineLabel('other'), 'other');
});
