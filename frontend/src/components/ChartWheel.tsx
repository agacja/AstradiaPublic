import { useState } from 'react';
import { SIGN_NAMES, SIGN_PATHS, PLANET_PATHS, PlanetGlyph } from './Glyph';

// Deliberately hand-authored display positions. These are not a person's chart,
// market data, a saved model output, or an astronomical calculation.
const illustration = [
  { name: 'Sun', longitude: 28 }, { name: 'Moon', longitude: 74 },
  { name: 'Mercury', longitude: 12 }, { name: 'Venus', longitude: 116 },
  { name: 'Mars', longitude: 159 }, { name: 'Jupiter', longitude: 207 },
  { name: 'Saturn', longitude: 244 }, { name: 'Uranus', longitude: 278 },
  { name: 'Neptune', longitude: 307 }, { name: 'Pluto', longitude: 344 },
];
const point = (degrees: number, radius: number) => {
  const angle = (180 - degrees) * Math.PI / 180;
  return { x: 250 + radius * Math.cos(angle), y: 250 - radius * Math.sin(angle) };
};

export function ChartWheel() {
  const [selected, setSelected] = useState(illustration[0]);
  return <div className="chart-grid">
    <div className="wheel-wrap">
      <svg viewBox="0 0 500 500" role="img" aria-label="Illustrative zodiac wheel with synthetic planet positions">
        <circle cx="250" cy="250" r="231" fill="#f7f7ff" stroke="#cbd0ee" />
        {[198, 153, 104].map(r => <circle key={r} cx="250" cy="250" r={r} fill="none" stroke="#d9ddf3" />)}
        {SIGN_NAMES.map((name, i) => {
          const start = point(i * 30, 153), end = point(i * 30, 231), label = point(i * 30 + 15, 214);
          return <g key={name}><line x1={start.x} y1={start.y} x2={end.x} y2={end.y} stroke="#d9ddf3" />
            <path d={SIGN_PATHS[i]} transform={`translate(${label.x} ${label.y})`} fill="none" stroke="#6366a8" strokeWidth="1.5" strokeLinecap="round" />
          </g>;
        })}
        {illustration.map((planet, i) => {
          const p = point(planet.longitude, i % 2 ? 175 : 132);
          return <g key={planet.name} transform={`translate(${p.x} ${p.y})`}>
            <circle r="17" fill={selected.name === planet.name ? '#dedfff' : '#fff'} stroke="#d5d8ef" />
            <path d={PLANET_PATHS[planet.name]} fill="none" stroke="#5147a5" strokeWidth="1.5" strokeLinecap="round" />
          </g>;
        })}
        <text x="250" y="245" textAnchor="middle" className="wheel-center">ASTRADIA</text>
        <text x="250" y="267" textAnchor="middle" className="wheel-sub">DISPLAY STUDY</text>
      </svg>
    </div>
    <div className="positions">
      <div className="eyebrow">Explore the interface</div><h3>Planetary positions</h3>
      <p>Choose a planet to inspect its display position.</p>
      <div className="planet-list">{illustration.map(p => <button key={p.name} onClick={() => setSelected(p)} aria-pressed={selected.name === p.name}>
        <PlanetGlyph name={p.name} size={18} /><span>{p.name}</span><span className="mono">{p.longitude}°</span>
      </button>)}</div>
      <div className="selected-planet"><strong>{selected.name}</strong><span>{SIGN_NAMES[Math.floor(selected.longitude / 30)]} · {selected.longitude % 30}°</span></div>
      <p className="fine">Synthetic display fixture. No birth data or private calculations.</p>
    </div>
  </div>;
}
