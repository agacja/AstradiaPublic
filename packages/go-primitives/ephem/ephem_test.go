package ephem

import (
	"math"
	"testing"
	"time"
)

// Reference longitudes from Swiss Ephemeris for the same instants.
func TestAgainstSwissReference(t *testing.T) {
	cases := []struct {
		when  time.Time
		body  string
		swiss float64
		tol   float64
	}{
		{time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC), "Sun", 140.048, 0.03},
		{time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC), "Saturn", 14.508, 0.15},
		{time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), "Saturn", 14.749, 0.15},
		{time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), "Jupiter", 126.076, 0.15},
		{time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), "Moon", 287.047, 0.15},
		{time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), "Mercury", 107.115, 0.2},
		{time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), "Venus", 169.872, 0.15},
		{time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), "Mars", 80.356, 0.15},
		{time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), "Neptune", 4.306, 0.15},
		{time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), "Pluto", 304.269, 0.25},
	}
	for _, c := range cases {
		got := Longitudes(c.when)[c.body]
		diff := math.Abs(got - c.swiss)
		if diff > 180 {
			diff = 360 - diff
		}
		if diff > c.tol {
			t.Errorf("%s @ %s: go=%.3f swiss=%.3f diff=%.3f > tol %.2f", c.body, c.when.Format("2006-01-02"), got, c.swiss, diff, c.tol)
		} else {
			t.Logf("%s: diff %.3f° (tol %.2f) OK", c.body, diff, c.tol)
		}
	}
}
