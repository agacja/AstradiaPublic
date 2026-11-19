import assert from 'node:assert/strict';
import test from 'node:test';
import { calculateAstrocartography, getLineLabel } from '../src/utils/astrocartography.ts';

const chart = calculateAstrocartography(new Date('2000-01-01T12:00:00Z'), 51.5074, -0.1278);
const byPlanet = (planet: string, type: string) => chart.lines.find(l => l.planet === planet && l.lineType === type);
const wrap = (lon: number) => ((((lon + 180) % 360) + 360) % 360) - 180;

test('every body has a meridian pair exactly half a turn apart', () => {
  for (const planet of ['Sun', 'Moon', 'Mercury', 'Venus', 'Mars', 'Jupiter', 'Saturn', 'Uranus', 'Neptune', 'Pluto']) {
    const mc = byPlanet(planet, 'MC');
    const ic = byPlanet(planet, 'IC');
    assert.ok(mc && ic, planet);
    const mcLon = mc.points[0].longitude;
    assert.ok(mc.points.every(p => p.longitude === mcLon), `${planet} MC is a meridian`);
    assert.ok(Math.abs(wrap(ic.points[0].longitude - mcLon - 180)) < 1e-9, `${planet} IC opposes MC`);
  }
});

test('the Sun culminates near Greenwich at Greenwich noon', () => {
  // 2000-01-01 12:00 UTC: the equation of time is about -3 minutes, under one degree.
  const mc = byPlanet('Sun', 'MC');
  assert.ok(mc);
  assert.ok(Math.abs(mc.points[0].longitude) < 1.5, `Sun MC at ${mc.points[0].longitude}`);
});

test('horizon curves stay on the map and are ordered for drawing', () => {
  const curves = chart.lines.filter(l => l.lineType === 'ASC' || l.lineType === 'DSC');
  assert.ok(curves.length >= 18);
  for (const line of curves) {
    assert.ok(line.points.length > 1);
    for (let i = 0; i < line.points.length; i++) {
      const p = line.points[i];
      assert.ok(Math.abs(p.latitude) <= 75 && Math.abs(p.longitude) <= 180);
      if (i > 0) assert.ok(p.longitude >= line.points[i - 1].longitude);
    }
  }
});

test('the birth location is echoed and labels are stable', () => {
  assert.deepEqual(chart.birthLocation, { latitude: 51.5074, longitude: -0.1278 });
  assert.equal(getLineLabel('MC'), 'Midheaven');
  assert.equal(getLineLabel('other'), 'other');
});
