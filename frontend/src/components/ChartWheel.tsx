import { useState } from 'react';
import { SIGN_NAMES, SIGN_PATHS, PLANET_PATHS, PlanetGlyph } from './Glyph';

interface DisplayPlanet {
  readonly name: string;
  /** Degrees along the zodiac, 0 at the start of Aries. */
  readonly longitude: number;
}

// Deliberately hand-authored display positions. These are not a person's chart,
// market data, a saved model output, or an astronomical calculation.
const ILLUSTRATION = [
  { name: 'Sun', longitude: 28 },
  { name: 'Moon', longitude: 74 },
  { name: 'Mercury', longitude: 12 },
  { name: 'Venus', longitude: 116 },
  { name: 'Mars', longitude: 159 },
  { name: 'Jupiter', longitude: 207 },
  { name: 'Saturn', longitude: 244 },
  { name: 'Uranus', longitude: 278 },
  { name: 'Neptune', longitude: 307 },
  { name: 'Pluto', longitude: 344 },
] as const satisfies readonly [DisplayPlanet, ...DisplayPlanet[]];

// The wheel is drawn in a square box around its centre; radii are in the same units.
const SIZE = 500;
const CENTRE = SIZE / 2;
const RIM_RADIUS = 231;
const SIGN_BAND_INNER_RADIUS = 153;
const RING_RADII = [198, SIGN_BAND_INNER_RADIUS, 104] as const;
const SIGN_GLYPH_RADIUS = 214;
// Planets alternate between two radii, so neighbours on the wheel do not overlap.
const INNER_PLANET_RADIUS = 132;
const OUTER_PLANET_RADIUS = 175;
const DEGREES_PER_SIGN = 30;

/** A point on the wheel. Longitude 0 is on the left and grows clockwise, over the top. */
function point(degrees: number, radius: number): { x: number; y: number } {
  const angle = ((180 - degrees) * Math.PI) / 180;
  return { x: CENTRE + radius * Math.cos(angle), y: CENTRE - radius * Math.sin(angle) };
}

export function ChartWheel() {
  const [selected, setSelected] = useState<DisplayPlanet>(ILLUSTRATION[0]);
  return (
    <div className="chart-grid">
      <div className="wheel-wrap">
        <svg viewBox={`0 0 ${SIZE} ${SIZE}`} role="img" aria-label="Illustrative zodiac wheel with synthetic planet positions">
          <circle cx={CENTRE} cy={CENTRE} r={RIM_RADIUS} fill="#f7f7ff" stroke="#cbd0ee" />
          {RING_RADII.map(radius => (
            <circle key={radius} cx={CENTRE} cy={CENTRE} r={radius} fill="none" stroke="#d9ddf3" />
          ))}
          {SIGN_NAMES.map((name, i) => {
            const start = point(i * DEGREES_PER_SIGN, SIGN_BAND_INNER_RADIUS);
            const end = point(i * DEGREES_PER_SIGN, RIM_RADIUS);
            const label = point((i + 0.5) * DEGREES_PER_SIGN, SIGN_GLYPH_RADIUS);
            return (
              <g key={name}>
                <line x1={start.x} y1={start.y} x2={end.x} y2={end.y} stroke="#d9ddf3" />
                <path
                  d={SIGN_PATHS[i]}
                  transform={`translate(${label.x} ${label.y})`}
                  fill="none"
                  stroke="#6366a8"
                  strokeWidth="1.5"
                  strokeLinecap="round"
                />
              </g>
            );
          })}
          {ILLUSTRATION.map((planet, i) => {
            const at = point(planet.longitude, i % 2 === 0 ? INNER_PLANET_RADIUS : OUTER_PLANET_RADIUS);
            return (
              <g key={planet.name} transform={`translate(${at.x} ${at.y})`}>
                <circle r="17" fill={selected.name === planet.name ? '#dedfff' : '#fff'} stroke="#d5d8ef" />
                <path d={PLANET_PATHS[planet.name]} fill="none" stroke="#5147a5" strokeWidth="1.5" strokeLinecap="round" />
              </g>
            );
          })}
          <text x={CENTRE} y={CENTRE - 5} textAnchor="middle" className="wheel-center">ASTRADIA</text>
          <text x={CENTRE} y={CENTRE + 17} textAnchor="middle" className="wheel-sub">DISPLAY STUDY</text>
        </svg>
      </div>
      <div className="positions">
        <div className="eyebrow">Explore the interface</div>
        <h3>Planetary positions</h3>
        <p>Choose a planet to inspect its display position.</p>
        <div className="planet-list">
          {ILLUSTRATION.map(planet => (
            <button key={planet.name} onClick={() => setSelected(planet)} aria-pressed={selected.name === planet.name}>
              <PlanetGlyph name={planet.name} size={18} />
              <span>{planet.name}</span>
              <span className="mono">{planet.longitude}°</span>
            </button>
          ))}
        </div>
        <div className="selected-planet">
          <strong>{selected.name}</strong>
          <span>{SIGN_NAMES[Math.floor(selected.longitude / DEGREES_PER_SIGN)]} · {selected.longitude % DEGREES_PER_SIGN}°</span>
        </div>
        <p className="fine">Synthetic display fixture. No birth data or private calculations.</p>
      </div>
    </div>
  );
}
