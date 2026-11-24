package socialstore

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFriendPairLockKey(t *testing.T) {
	// Pinned: every running instance must derive the same key for a pair, so
	// a change to the encoding is a change to the locking protocol and has
	// to be made deliberately.
	if got := friendPairLockKey("ann", "bob"); got != -4790991055385473868 {
		t.Fatalf("lock key for (ann,bob) changed: %d", got)
	}
	if friendPairLockKey("ann", "bob") != friendPairLockKey("bob", "ann") {
		t.Fatal("lock key depends on the direction of the request")
	}
	distinct := map[int64][2]string{}
	for _, pair := range [][2]string{
		{"ann", "bob"},
		{"ann", "bo"},
		{"an", "nbob"},
		{"annb", "ob"}, // same concatenation as (ann,bob), different pair
		{"ann", "cyd"},
		{"bob", "cyd"},
	} {
		key := friendPairLockKey(pair[0], pair[1])
		if other, ok := distinct[key]; ok {
			t.Fatalf("pairs %v and %v share a lock key", other, pair)
		}
		distinct[key] = pair
	}
}

func TestBirthUTCAppliesTheOffsetInMinutes(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name   string
		date   string
		clock  string
		offset *float64
		want   string // UTC, "2006-01-02 15:04"
	}{
		// Fractional zones: the half or three-quarter hour must survive.
		{"India +5.5", "1994-03-21", "10:30", f(5.5), "1994-03-21 05:00"},
		{"Nepal +5.75", "1994-03-21", "10:30", f(5.75), "1994-03-21 04:45"},
		{"Newfoundland -3.5", "1994-03-21", "10:30", f(-3.5), "1994-03-21 14:00"},
		{"Adelaide +9.5", "1994-03-21", "10:30", f(9.5), "1994-03-21 01:00"},
		{"Marquesas -9.5", "1994-03-21", "10:30", f(-9.5), "1994-03-21 20:00"},
		// Legacy rows: minutes, opposite sign (beyond ±14 means minutes).
		{"legacy -330 minutes is +5.5", "1994-03-21", "10:30", f(-330), "1994-03-21 05:00"},
		{"legacy -345 minutes is +5.75", "1994-03-21", "10:30", f(-345), "1994-03-21 04:45"},
		{"legacy 210 minutes is -3.5", "1994-03-21", "10:30", f(210), "1994-03-21 14:00"},
		{"legacy 480 minutes is -8", "1994-03-21", "10:30", f(480), "1994-03-21 18:30"},
		{"legacy -60 minutes is +1", "1994-03-21", "10:30", f(-60), "1994-03-21 09:30"},
		// Whole hours behave exactly as before.
		{"UTC", "1994-03-21", "10:30", f(0), "1994-03-21 10:30"},
		{"no offset stored", "1994-03-21", "10:30", nil, "1994-03-21 10:30"},
		{"+1", "1994-03-21", "10:30", f(1), "1994-03-21 09:30"},
		{"-8", "1994-03-21", "10:30", f(-8), "1994-03-21 18:30"},
		{"+14 crosses to the previous day", "1994-03-21", "10:30", f(14), "1994-03-20 20:30"},
		{"-12 crosses to the next day", "1994-03-21", "22:30", f(-12), "1994-03-22 10:30"},
		// Around the clock and the calendar.
		{"just after midnight at +5.5", "1994-03-21", "00:15", f(5.5), "1994-03-20 18:45"},
		{"new year at +5.75", "2000-01-01", "00:00", f(5.75), "1999-12-31 18:15"},
		{"unknown time is local noon", "1994-03-21", "", f(5.5), "1994-03-21 06:30"},
		{"seconds in the stored time are ignored", "1994-03-21", "10:30:59", f(5.5), "1994-03-21 05:00"},
		{"a timestamp-shaped date uses its day", "1994-03-21 00:00:00", "10:30", f(-3.5), "1994-03-21 14:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := birthUTC(tc.date, tc.clock, tc.offset)
			if err != nil {
				t.Fatal(err)
			}
			if s := got.Format("2006-01-02 15:04"); s != tc.want || got.Location() != time.UTC {
				t.Fatalf("got %s (%s), want %s UTC", s, got.Location(), tc.want)
			}
		})
	}

	// The old conversion truncated the hour difference (10 - 5.5 → 4) and
	// answered 04:30 here, half an hour early. Pin that it is gone.
	got, _ := birthUTC("1994-03-21", "10:30", f(5.5))
	if got.Format("15:04") == "04:30" {
		t.Fatal("the half hour of a +5.5 offset was dropped")
	}

	for _, bad := range []string{"", "not-a-date", "1994/03/21"} {
		if _, err := birthUTC(bad, "10:30", nil); err == nil {
			t.Errorf("date %q accepted", bad)
		}
	}
}

// The store refuses an oversized picture itself, before any query, whatever
// the handler in front of it checked.
func TestSetAvatarRefusesAPictureOverTheCap(t *testing.T) {
	var s Store // no pool: a refused picture must not reach the database
	err := s.SetAvatar(context.Background(), "someone", make([]byte, MaxAvatarBytes+1), "image/png")
	if !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("oversized picture: %v", err)
	}
}

// What each member of a pair is shown for every stored consent state. The
// word "declined" must never reach the member whose request it was.
func TestSynastryConsentSeenBy(t *testing.T) {
	for _, tc := range []struct {
		status, requestedBy string
		asker, answerer     string // what "ann" (who asked) and "bob" see
	}{
		{"none", "", "none", "none"},
		{"pending", "ann", "pending-out", "pending-in"},
		{"accepted", "ann", "accepted", "accepted"},
		{"declined", "ann", "pending-out", "declined"},
		{"withdrawn", "ann", "none", "declined"},
		{"something-else", "ann", "none", "none"},
	} {
		c := &SynastryConsent{Status: tc.status, RequestedBy: tc.requestedBy}
		if got := c.SeenBy("ann"); got != tc.asker {
			t.Errorf("%s: the member who asked sees %q, want %q", tc.status, got, tc.asker)
		}
		if got := c.SeenBy("bob"); got != tc.answerer {
			t.Errorf("%s: the other member sees %q, want %q", tc.status, got, tc.answerer)
		}
	}
}
