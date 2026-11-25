package socialstore

import (
	"context"
	"errors"
	"testing"
)

// Unread counts, now one statement: what arrived after each mark counts,
// what arrived before it does not, a member with no marks yet counts
// everything, and nothing of one's own counts.
func TestUnreadCountsCountWhatArrivedSinceEachMark(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "me", "a", "b", "c", "d")
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := fx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Friend requests to me: one old, two new; one I sent (not mine to answer).
	befriend(t, fx, "a", "me", "pending")
	befriend(t, fx, "b", "me", "pending")
	befriend(t, fx, "c", "me", "pending")
	befriend(t, fx, "me", "d", "pending")
	exec(`UPDATE friendships SET created_at = NOW() - INTERVAL '3 days' WHERE requester_id = 'a'`)
	// Synastry requests: one from b waiting for me (new), one I made (not mine).
	exec(`INSERT INTO synastry_consents (id, user_lo, user_hi, requested_by, status, created_at)
	      VALUES ('s1', 'b', 'me', 'b', 'pending', NOW()), ('s2', 'c', 'me', 'me', 'pending', NOW())`)
	// Wall: two comments by others (one old, one new), one by me.
	exec(`INSERT INTO profile_comments (id, profile_id, author_id, body, created_at) VALUES
	      ('w1', 'me', 'a', 'old', NOW() - INTERVAL '3 days'),
	      ('w2', 'me', 'b', 'new', NOW()),
	      ('w3', 'me', 'me', 'mine', NOW())`)

	want := func(when string, friends, synastry, wall int) {
		t.Helper()
		u, err := s.UnreadCounts(ctx, "me")
		if err != nil {
			t.Fatal(err)
		}
		if u.FriendRequests != friends || u.SynastryRequests != synastry || u.WallComments != wall ||
			u.Total != friends+synastry+wall {
			t.Fatalf("%s: %+v, want %d/%d/%d", when, u, friends, synastry, wall)
		}
	}
	want("never looked", 3, 1, 2)
	exec(`INSERT INTO social_seen (user_id, friends_seen_at, synastry_seen_at, wall_seen_at)
	      VALUES ('me', NOW() - INTERVAL '1 day', NULL, NOW() - INTERVAL '1 day')`)
	want("looked a day ago, never at synastry", 2, 1, 1)
	exec(`UPDATE social_seen SET friends_seen_at = NOW() + INTERVAL '1 minute',
	      synastry_seen_at = NOW() + INTERVAL '1 minute', wall_seen_at = NOW() + INTERVAL '1 minute'`)
	want("looked after everything", 0, 0, 0)
	if u, err := s.UnreadCounts(ctx, "nobody"); err != nil || u.Total != 0 {
		t.Fatalf("a member with nothing: %+v %v", u, err)
	}
}

// What a friend's dashboard reads of the owner's row comes from one
// statement, and only for an accepted friend.
func TestFriendProfileIsReadOnlyForAnAcceptedFriend(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "owner", "friend", "asked", "refused", "stranger", "bare", "bigpic")
	befriend(t, fx, "owner", "friend", "accepted")
	befriend(t, fx, "asked", "owner", "pending")
	befriend(t, fx, "owner", "refused", "declined")
	befriend(t, fx, "bare", "owner", "accepted")
	befriend(t, fx, "bigpic", "owner", "accepted")
	if _, err := fx.Exec(ctx, `
		UPDATE users SET nick = 'Owl', dashboard_prefs = '{"widgets":[]}', shared_status = '{"note":"hi"}',
		       avatar = $1, avatar_mime = 'image/png',
		       birth_date = '1990-05-17', birth_time = '14:30', birth_timezone_offset = 2
		WHERE id = 'owner'`, []byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.Exec(ctx, `UPDATE users SET avatar = $1, avatar_mime = 'image/png' WHERE id = 'bigpic'`,
		make([]byte, MaxAvatarBytes+1)); err != nil {
		t.Fatal(err)
	}

	for _, viewer := range []string{"asked", "refused", "stranger", "owner", "nobody"} {
		if p, err := s.FriendProfile(ctx, viewer, "owner"); !errors.Is(err, ErrNotFriends) || p != nil {
			t.Fatalf("%s reads owner: %+v %v, want ErrNotFriends", viewer, p, err)
		}
	}
	p, err := s.FriendProfile(ctx, "friend", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if p.Nick != "Owl" || string(p.Prefs) != `{"widgets": []}` || string(p.Status) != `{"note": "hi"}` ||
		p.Avatar != "data:image/png;base64,iVBORw==" || p.Birth == nil || p.Birth.UTC.Format("2006-01-02 15:04") != "1990-05-17 12:30" {
		t.Fatalf("friend reads owner: %+v (birth %v)", p, p.Birth)
	}
	// A friend with nothing saved reads as empty, with the default handle.
	p, err = s.FriendProfile(ctx, "owner", "bare")
	if err != nil || p.Nick != "seeker-bare" || p.Prefs != nil || p.Status != nil || p.Avatar != "" || p.Birth != nil {
		t.Fatalf("owner reads bare: %+v %v", p, err)
	}
	// A picture stored over the cap is not read out for anyone else.
	if p, err = s.FriendProfile(ctx, "owner", "bigpic"); err != nil || p.Avatar != "" {
		t.Fatalf("owner reads bigpic: avatar of %d characters, %v", len(p.Avatar), err)
	}
}

// The pair read behind a chart comparison: each part only as far as the one
// before it allows.
func TestSynastryPairReadsEachPartOnlyWhenAllowed(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "bob", "cyd", "dee")
	befriend(t, fx, "ann", "bob", "accepted")
	befriend(t, fx, "ann", "cyd", "accepted")
	if _, err := fx.Exec(ctx, `UPDATE users SET birth_date = '1990-05-17', birth_time = '14:30', birth_timezone_offset = 0
	                           WHERE id IN ('ann', 'bob', 'dee')`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.Exec(ctx, `UPDATE users SET nick = 'Bobby' WHERE id = 'bob'`); err != nil {
		t.Fatal(err)
	}

	// Not friends: nothing past the friendship.
	p, err := s.SynastryPair(ctx, "ann", "dee")
	if err != nil || p.Friends || p.Consent.Status != "none" || p.Nick != "" || p.Mine != nil || p.Theirs != nil {
		t.Fatalf("strangers: %+v %v", p, err)
	}
	// Friends, nobody asked: the nick, no charts.
	p, err = s.SynastryPair(ctx, "ann", "bob")
	if err != nil || !p.Friends || p.Consent.Status != "none" || p.Nick != "Bobby" || p.Mine != nil || p.Theirs != nil {
		t.Fatalf("friends, no consent: %+v %v", p, err)
	}
	// Asked, not yet answered: the request, still no charts.
	if res, err := s.RequestSynastry(ctx, "bob", "ann"); err != nil || res != "requested" {
		t.Fatalf("bob asks: %q %v", res, err)
	}
	p, err = s.SynastryPair(ctx, "ann", "bob")
	if err != nil || p.Consent.Status != "pending" || p.Consent.RequestedBy != "bob" || p.Mine != nil || p.Theirs != nil {
		t.Fatalf("pending: %+v %v", p, err)
	}
	// Both said yes: both charts, from either side.
	if ok, err := s.RespondSynastry(ctx, "ann", "bob", true); err != nil || !ok {
		t.Fatalf("ann accepts: %v %v", ok, err)
	}
	for _, side := range [][2]string{{"ann", "bob"}, {"bob", "ann"}} {
		p, err = s.SynastryPair(ctx, side[0], side[1])
		if err != nil || p.Consent.Status != "accepted" || p.Mine == nil || p.Theirs == nil {
			t.Fatalf("%s with %s, accepted: %+v %v", side[0], side[1], p, err)
		}
	}
	if p.Nick != "seeker-ann" {
		t.Fatalf("bob sees ann as %q", p.Nick)
	}
	// Granted, but one side never saved a chart.
	if _, err := fx.Exec(ctx, `INSERT INTO synastry_consents (id, user_lo, user_hi, requested_by, status)
	                          VALUES ('s', 'ann', 'cyd', 'ann', 'accepted')`); err != nil {
		t.Fatal(err)
	}
	p, err = s.SynastryPair(ctx, "ann", "cyd")
	if err != nil || p.Consent.Status != "accepted" || p.Mine == nil || p.Theirs != nil {
		t.Fatalf("cyd has no chart: %+v %v", p, err)
	}
}

// Which member of a pair is user_lo is decided by the database's collation,
// the same comparison the table's CHECK (user_lo < user_hi) makes. Go used to
// decide it by bytes; under a collation that sorts letters case-blind
// ("amy" < "Zed", while by bytes "Zed" < "amy") every synastry request of
// such a pair failed the CHECK.
func TestSynastryWorksForIdsThatSortDifferentlyByBytes(t *testing.T) {
	dsn, fx := collatedSchema(t, "en-US")
	s := openTestStore(t, dsn)
	ctx := context.Background()
	addUsers(t, fx, "amy", "Zed")
	befriend(t, fx, "amy", "Zed", "accepted")
	if _, err := fx.Exec(ctx, `UPDATE users SET birth_date = '1990-05-17', birth_time = '14:30', birth_timezone_offset = 0`); err != nil {
		t.Fatal(err)
	}
	var byBytes, byCollation bool
	if err := fx.QueryRow(ctx, `SELECT 'Zed' COLLATE "C" < 'amy' COLLATE "C", 'amy'::varchar < 'Zed'::varchar`).Scan(&byBytes, &byCollation); err != nil {
		t.Fatal(err)
	}
	if !byBytes || !byCollation {
		t.Fatal("the fixture must sort one way by bytes and the other by the database's collation")
	}

	if res, err := s.RequestSynastry(ctx, "Zed", "amy"); err != nil || res != "requested" {
		t.Fatalf("Zed asks amy: %q %v", res, err)
	}
	if p, err := s.SynastryPair(ctx, "amy", "Zed"); err != nil || p.Consent.Status != "pending" || p.Consent.RequestedBy != "Zed" {
		t.Fatalf("amy reads the request: %+v %v", p, err)
	}
	if ok, err := s.RespondSynastry(ctx, "amy", "Zed", true); err != nil || !ok {
		t.Fatalf("amy accepts: %v %v", ok, err)
	}
	if p, err := s.SynastryPair(ctx, "Zed", "amy"); err != nil || p.Consent.Status != "accepted" || p.Mine == nil || p.Theirs == nil {
		t.Fatalf("Zed reads the accepted pair: %+v %v", p, err)
	}
	if u, err := s.UnreadCounts(ctx, "amy"); err != nil || u.SynastryRequests != 0 {
		t.Fatalf("amy's unread: %+v %v", u, err)
	}
	if ok, err := s.RevokeSynastry(ctx, "amy", "Zed"); err != nil || !ok {
		t.Fatalf("amy revokes: %v %v", ok, err)
	}
	if res, err := s.RequestSynastry(ctx, "amy", "Zed"); err != nil || res != "requested" {
		t.Fatalf("amy asks again: %q %v", res, err)
	}
	if ok, err := s.RemoveFriend(ctx, "Zed", "amy"); err != nil || !ok {
		t.Fatalf("unfriend: %v %v", ok, err)
	}
	var left int
	if err := fx.QueryRow(ctx, `SELECT COUNT(*) FROM synastry_consents`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("the consent outlived the friendship: %d rows, %v", left, err)
	}
}
