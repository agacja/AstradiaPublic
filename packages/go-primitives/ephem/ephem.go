// Package ephem computes geocentric ecliptic longitudes for the ten
// astrological bodies in pure Go — no cgo, no WASM, no network.
//
// Method: JPL "Keplerian Elements for Approximate Positions of the Major
// Planets" (Standish), 1800 AD – 2050 AD table, plus a truncated Meeus ch. 47
// series for the Moon. Accuracy inside that window is a few hundredths of a
// degree for the Sun, about a tenth for the Moon and the planets, and a few
// tenths worst-case for Pluto; the test checks the Sun, the Moon and seven
// planets against reference values within those tolerances.
// A separate Swiss Ephemeris service supplies full-precision charts; this
// package exists so the social service has zero runtime dependencies.
package ephem

import (
	"math"
	"time"
)

const rad = math.Pi / 180

func norm360(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}
	return d
}

// JulianDay for a UTC instant.
func JulianDay(t time.Time) float64 {
	t = t.UTC()
	y, m := t.Year(), int(t.Month())
	d := float64(t.Day()) + (float64(t.Hour())+float64(t.Minute())/60+float64(t.Second())/3600)/24
	if m <= 2 {
		y--
		m += 12
	}
	a := y / 100
	b := 2 - a + a/4
	return math.Floor(365.25*float64(y+4716)) + math.Floor(30.6001*float64(m+1)) + d + float64(b) - 1524.5
}

// Keplerian elements at J2000 + centennial rates (JPL approximate positions,
// valid 1800–2050): a (au), e, I (deg), L (deg), longPeri ϖ (deg), longNode Ω (deg).
type elements struct {
	a, e, i, l, w, o       float64
	da, de, di, dl, dw, do float64
}

var planetElements = map[string]elements{
	"Mercury": {0.38709927, 0.20563593, 7.00497902, 252.25032350, 77.45779628, 48.33076593,
		0.00000037, 0.00001906, -0.00594749, 149472.67411175, 0.16047689, -0.12534081},
	"Venus": {0.72333566, 0.00677672, 3.39467605, 181.97909950, 131.60246718, 76.67984255,
		0.00000390, -0.00004107, -0.00078890, 58517.81538729, 0.00268329, -0.27769418},
	"Earth": {1.00000261, 0.01671123, -0.00001531, 100.46457166, 102.93768193, 0.0,
		0.00000562, -0.00004392, -0.01294668, 35999.37244981, 0.32327364, 0.0},
	"Mars": {1.52371034, 0.09339410, 1.84969142, -4.55343205, -23.94362959, 49.55953891,
		0.00001847, 0.00007882, -0.00813131, 19140.30268499, 0.44441088, -0.29257343},
	"Jupiter": {5.20288700, 0.04838624, 1.30439695, 34.39644051, 14.72847983, 100.47390909,
		-0.00011607, -0.00013253, -0.00183714, 3034.74612775, 0.21252668, 0.20469106},
	"Saturn": {9.53667594, 0.05386179, 2.48599187, 49.95424423, 92.59887831, 113.66242448,
		-0.00125060, -0.00050991, 0.00193609, 1222.49362201, -0.41897216, -0.28867794},
	"Uranus": {19.18916464, 0.04725744, 0.77263783, 313.23810451, 170.95427630, 74.01692503,
		-0.00196176, -0.00004397, -0.00242939, 428.48202785, 0.40805281, 0.04240589},
	"Neptune": {30.06992276, 0.00859048, 1.77004347, -55.12002969, 44.96476227, 131.78422574,
		0.00026291, 0.00005105, 0.00035372, 218.45945325, -0.32241464, -0.00508664},
	"Pluto": {39.48211675, 0.24882730, 17.14001206, 238.92903833, 224.06891629, 110.30393684,
		-0.00031596, 0.00005170, 0.00004818, 145.20780515, -0.04062942, -0.01183482},
}

// heliocentric ecliptic rectangular coordinates (au) of a body at Julian
// centuries T from J2000.
func helio(el elements, T float64) (x, y, z float64) {
	a := el.a + el.da*T
	e := el.e + el.de*T
	i := (el.i + el.di*T) * rad
	L := norm360(el.l + el.dl*T)
	w := el.w + el.dw*T // longitude of perihelion ϖ
	o := el.o + el.do*T // longitude of ascending node Ω

	M := norm360(L-w) * rad // mean anomaly
	argPeri := (w - o) * rad
	node := o * rad

	// Kepler's equation, Newton iterations
	E := M
	for k := 0; k < 8; k++ {
		E = E - (E-e*math.Sin(E)-M)/(1-e*math.Cos(E))
	}

	// position in orbital plane
	xv := a * (math.Cos(E) - e)
	yv := a * math.Sqrt(1-e*e) * math.Sin(E)

	// rotate: argument of perihelion, inclination, node
	cosw, sinw := math.Cos(argPeri), math.Sin(argPeri)
	coso, sino := math.Cos(node), math.Sin(node)
	cosi, sini := math.Cos(i), math.Sin(i)

	xh := (cosw*coso-sinw*sino*cosi)*xv + (-sinw*coso-cosw*sino*cosi)*yv
	yh := (cosw*sino+sinw*coso*cosi)*xv + (-sinw*sino+cosw*coso*cosi)*yv
	zh := (sinw*sini)*xv + (cosw*sini)*yv
	return xh, yh, zh
}

// moonLongitude — truncated Meeus ch. 47 series.
func moonLongitude(T float64) float64 {
	Lp := norm360(218.3164477 + 481267.88123421*T - 0.0015786*T*T)
	D := norm360(297.8501921+445267.1114034*T-0.0018819*T*T) * rad
	M := norm360(357.5291092+35999.0502909*T-0.0001536*T*T) * rad
	Mp := norm360(134.9633964+477198.8675055*T+0.0087414*T*T) * rad
	F := norm360(93.2720950+483202.0175233*T-0.0036539*T*T) * rad

	lon := Lp +
		6.288774*math.Sin(Mp) +
		1.274027*math.Sin(2*D-Mp) +
		0.658314*math.Sin(2*D) +
		0.213618*math.Sin(2*Mp) -
		0.185116*math.Sin(M) -
		0.114332*math.Sin(2*F) +
		0.058793*math.Sin(2*D-2*Mp) +
		0.057066*math.Sin(2*D-M-Mp) +
		0.053322*math.Sin(2*D+Mp) +
		0.045758*math.Sin(2*D-M) -
		0.040923*math.Sin(M-Mp) -
		0.034720*math.Sin(D) -
		0.030383*math.Sin(M+Mp) +
		0.015327*math.Sin(2*D-2*F) -
		0.012528*math.Sin(Mp+2*F) +
		0.010980*math.Sin(Mp-2*F)
	return norm360(lon)
}

// Bodies in the order the rest of the app uses.
var Bodies = []string{"Sun", "Moon", "Mercury", "Venus", "Mars", "Jupiter", "Saturn", "Uranus", "Neptune", "Pluto"}

// Longitudes returns geocentric ecliptic longitudes (degrees) for the ten
// bodies at a UTC instant.
func Longitudes(t time.Time) map[string]float64 {
	T := (JulianDay(t) - 2451545.0) / 36525.0
	out := make(map[string]float64, 10)

	// The JPL elements are referenced to the J2000 ecliptic/equinox; tropical
	// longitudes are measured from the equinox OF DATE. General precession
	// (Meeus 21.6, leading terms) closes the gap — without this every body
	// sits a constant ~50.3″/yr behind Swiss.
	prec := 1.3969713*T + 0.0003086*T*T

	ex, ey, _ := helio(planetElements["Earth"], T)

	// Sun: geocentric = anti-Earth
	out["Sun"] = norm360(math.Atan2(-ey, -ex)/rad + prec)
	// The Moon series already uses mean elements of date — no correction.
	out["Moon"] = moonLongitude(T)

	for name, el := range planetElements {
		if name == "Earth" {
			continue
		}
		px, py, _ := helio(el, T)
		gx, gy := px-ex, py-ey
		out[name] = norm360(math.Atan2(gy, gx)/rad + prec)
	}
	return out
}
