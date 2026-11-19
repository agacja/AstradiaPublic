// Display message types only. No calculation or scoring implementation.
export interface PlanetPosition {
  name: string;
  symbol: string;
  longitude: number;
  sign: string;
  signSymbol: string;
  degree: number;
  minute: number;
  house: number;
  retrograde: boolean;
}

export interface HouseData {
  number: number;
  sign: string;
  signSymbol: string;
  degree: number;
  cusp: number; // Store the actual cusp longitude
  planets: PlanetPosition[];
}

export interface NatalChart {
  sun: PlanetPosition;
  moon: PlanetPosition;
  mercury: PlanetPosition;
  venus: PlanetPosition;
  mars: PlanetPosition;
  jupiter: PlanetPosition;
  saturn: PlanetPosition;
  uranus: PlanetPosition;
  neptune: PlanetPosition;
  pluto: PlanetPosition;
  ascendant: number;
  midheaven: number;
  houses: HouseData[];
  houseCusps: number[]; // Store raw house cusps for transit calculations
  aspects: Aspect[];
  birthLatitude: number;
  birthLongitude: number;
}

export interface Aspect {
  planet1: string;
  planet2: string;
  type: string;
  angle: number;
  orb: number;
  symbol: string;
}
