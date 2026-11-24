package socialstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func edgeCount(t *testing.T, pool *pgxpool.Pool, a, b string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM friendships
		 WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)`,
		a, b).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Many members of the same pair ask at the same instant, in both directions.
// Exactly one edge may exist afterwards, every caller gets an answer, and
// nobody gets an error (the old check-then-insert gave a unique violation for
// a double click and two rows for crossed requests).
func TestCreateFriendRequestIsRaceFree(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()

	const rounds, callers = 25, 12
	for round := 0; round < rounds; round++ {
		a, b := fmt.Sprintf("a%02d", round), fmt.Sprintf("b%02d", round)
		addUsers(t, fx, a, b)

		results := make([]string, callers)
		errs := make([]error, callers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				from, to := a, b
				if i%2 == 1 {
					from, to = b, a
				}
				<-start
				results[i], errs[i] = s.CreateFriendRequest(ctx, from, to)
			}(i)
		}
		close(start)
		wg.Wait()

		created := 0
		for i := range results {
			if errs[i] != nil {
				t.Fatalf("round %d caller %d: %v", round, i, errs[i])
			}
			switch results[i] {
			case "created":
				created++
			case "exists":
			default:
				t.Fatalf("round %d caller %d: unexpected result %q", round, i, results[i])
			}
		}
		if created != 1 {
			t.Fatalf("round %d: %d callers were told they created the request, want 1", round, created)
		}
		if n := edgeCount(t, fx, a, b); n != 1 {
			t.Fatalf("round %d: %d friendship rows for one pair, want 1", round, n)
		}
	}
}

func TestCreateFriendRequestOutcomes(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "bob", "cyd")

	if _, err := s.CreateFriendRequest(ctx, "ann", "ann"); err != ErrSelfFriend {
		t.Fatalf("self request: %v", err)
	}
	for _, step := range []struct {
		from, to, want string
	}{
		{"ann", "bob", "created"},
		{"ann", "bob", "exists"}, // same direction again
		{"bob", "ann", "exists"}, // crossed
	} {
		got, err := s.CreateFriendRequest(ctx, step.from, step.to)
		if err != nil || got != step.want {
			t.Fatalf("%s→%s: %q, %v (want %q)", step.from, step.to, got, err, step.want)
		}
	}

	// Bob declines Ann's request.
	var requestID string
	if err := fx.QueryRow(ctx, `SELECT id FROM friendships WHERE requester_id = 'ann' AND addressee_id = 'bob'`).Scan(&requestID); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.RespondFriendRequest(ctx, requestID, "bob", false); err != nil || !ok {
		t.Fatalf("decline: %v %v", ok, err)
	}
	edge := func() (requester, status string, responded bool) {
		t.Helper()
		var at *time.Time
		if err := fx.QueryRow(ctx,
			`SELECT requester_id, status, responded_at FROM friendships
			 WHERE (requester_id = 'ann' AND addressee_id = 'bob') OR (requester_id = 'bob' AND addressee_id = 'ann')`).
			Scan(&requester, &status, &at); err != nil {
			t.Fatal(err)
		}
		return requester, status, at != nil
	}
	// Ann, who was declined, cannot simply ask again: for a week the answer
	// is the one a waiting request gets, and nothing changes.
	if got, err := s.CreateFriendRequest(ctx, "ann", "bob"); err != nil || got != "exists" {
		t.Fatalf("ann asks again at once: %q, %v", got, err)
	}
	if requester, status, responded := edge(); requester != "ann" || status != "declined" || !responded {
		t.Fatalf("edge after ann asked again: %s %s %v", requester, status, responded)
	}
	// After the week she may, and Bob gets a fresh request from her.
	if _, err := fx.Exec(ctx, `UPDATE friendships SET responded_at = NOW() - INTERVAL '6 days 23 hours' WHERE id = $1`, requestID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CreateFriendRequest(ctx, "ann", "bob"); err != nil || got != "exists" {
		t.Fatalf("ann asks just inside the week: %q, %v", got, err)
	}
	if _, err := fx.Exec(ctx, `UPDATE friendships SET responded_at = NOW() - INTERVAL '7 days 1 minute' WHERE id = $1`, requestID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CreateFriendRequest(ctx, "ann", "bob"); err != nil || got != "created" {
		t.Fatalf("ann asks after the week: %q, %v", got, err)
	}
	if requester, status, responded := edge(); requester != "ann" || status != "pending" || responded {
		t.Fatalf("edge after ann asked again: %s %s %v", requester, status, responded)
	}
	if n := edgeCount(t, fx, "ann", "bob"); n != 1 {
		t.Fatalf("%d rows after a reopened request, want 1", n)
	}
	// Bob declines again, then changes his mind: the one who declined may ask
	// in his own turn at any time, and the request is now his to Ann.
	if ok, err := s.RespondFriendRequest(ctx, requestID, "bob", false); err != nil || !ok {
		t.Fatalf("second decline: %v %v", ok, err)
	}
	if got, err := s.CreateFriendRequest(ctx, "bob", "ann"); err != nil || got != "created" {
		t.Fatalf("bob asks after declining: %q, %v", got, err)
	}
	if requester, status, responded := edge(); requester != "bob" || status != "pending" || responded {
		t.Fatalf("edge after bob asked: %s %s %v", requester, status, responded)
	}
	if n := edgeCount(t, fx, "ann", "bob"); n != 1 {
		t.Fatalf("%d rows after the decliner asked, want 1", n)
	}
	// It is a new request, from Bob, under a new id; the old id answers
	// nothing any more.
	declinedID := requestID
	if err := fx.QueryRow(ctx, `SELECT id FROM friendships WHERE requester_id = 'bob' AND addressee_id = 'ann'`).Scan(&requestID); err != nil || requestID == declinedID {
		t.Fatalf("bob's request has id %q (ann's declined one had %q), %v", requestID, declinedID, err)
	}
	if ok, err := s.RespondFriendRequest(ctx, declinedID, "ann", true); err != nil || ok {
		t.Fatalf("the id of the declined request still answers: %v %v", ok, err)
	}
	// Only Ann can answer it now, and a waiting request still blocks both.
	if ok, err := s.RespondFriendRequest(ctx, requestID, "bob", true); err != nil || ok {
		t.Fatalf("bob answered his own request: %v %v", ok, err)
	}
	for _, pair := range [][2]string{{"ann", "bob"}, {"bob", "ann"}} {
		if got, err := s.CreateFriendRequest(ctx, pair[0], pair[1]); err != nil || got != "exists" {
			t.Fatalf("while pending %s→%s: %q, %v", pair[0], pair[1], got, err)
		}
	}
	if ok, err := s.RespondFriendRequest(ctx, requestID, "ann", true); err != nil || !ok {
		t.Fatalf("ann accepts: %v %v", ok, err)
	}
	// An accepted friendship is never reopened by a request.
	for _, pair := range [][2]string{{"ann", "bob"}, {"bob", "ann"}} {
		if got, err := s.CreateFriendRequest(ctx, pair[0], pair[1]); err != nil || got != "exists" {
			t.Fatalf("while friends %s→%s: %q, %v", pair[0], pair[1], got, err)
		}
	}
	if _, status, _ := edge(); status != "accepted" {
		t.Fatalf("friendship became %s", status)
	}

	// Removing the edge frees the pair again.
	if ok, err := s.RemoveFriend(ctx, "bob", "ann"); err != nil || !ok {
		t.Fatalf("remove: %v %v", ok, err)
	}
	if got, err := s.CreateFriendRequest(ctx, "bob", "ann"); err != nil || got != "created" {
		t.Fatalf("after remove: %q, %v", got, err)
	}

	// A request to an account that does not exist is an error for the caller
	// to map, not a panic and not a phantom row.
	if _, err := s.CreateFriendRequest(ctx, "cyd", "ghost"); err == nil {
		t.Fatal("request to a missing account succeeded")
	}
	if n := edgeCount(t, fx, "cyd", "ghost"); n != 0 {
		t.Fatalf("%d rows for a missing account", n)
	}
}

// Top 8 ids are whatever the owner once sent. Only the ones that are accepted
// friends of the owner right now may be resolved to a nick and an avatar.
func TestUsersBriefResolvesOnlyAcceptedFriendsOfTheOwner(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()

	addUsers(t, fx, "owner", "f_out", "f_in", "pend_out", "pend_in", "declined", "stranger", "fof")
	befriend(t, fx, "owner", "f_out", "accepted")
	befriend(t, fx, "f_in", "owner", "accepted")
	befriend(t, fx, "owner", "pend_out", "pending")
	befriend(t, fx, "pend_in", "owner", "pending")
	befriend(t, fx, "owner", "declined", "declined")
	befriend(t, fx, "f_out", "fof", "accepted") // a friend's friend is not the owner's friend
	if _, err := fx.Exec(ctx, `UPDATE users SET nick = 'Inny', avatar = $1, avatar_mime = 'image/png' WHERE id = 'f_in'`, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.Exec(ctx, `UPDATE users SET nick = 'Stranger', avatar = $1, avatar_mime = 'image/png' WHERE id = 'stranger'`, []byte{9, 9, 9}); err != nil {
		t.Fatal(err)
	}

	ids := []string{"stranger", "f_in", "pend_out", "f_out", "declined", "pend_in", "fof", "owner", "ghost"}
	ids8 := func(rows []FriendEntry) []string {
		out := []string{}
		for _, r := range rows {
			out = append(out, r.UserID)
		}
		return out
	}

	rows, err := s.UsersBrief(ctx, "owner", ids)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(ids8(rows)); got != "[f_in f_out]" {
		t.Fatalf("resolved %s, want only the accepted friends in the stored order", got)
	}
	if rows[0].Nick != "Inny" || rows[0].Avatar != "data:image/png;base64,AQID" {
		t.Fatalf("friend row lost its display fields: %+v", rows[0])
	}

	friends, err := s.AcceptedFriendIDs(ctx, "owner", ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) != 2 || !friends["f_in"] || !friends["f_out"] {
		t.Fatalf("accepted friends: %v", friends)
	}

	// The same ids seen from someone else's profile resolve against THAT
	// owner's friendships, not the first one's.
	rows, err = s.UsersBrief(ctx, "f_out", ids)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(ids8(rows)); got != "[fof owner]" {
		t.Fatalf("resolved %s for f_out", got)
	}

	// Ending the friendship ends the resolution, with the id still stored.
	if ok, err := s.RemoveFriend(ctx, "f_out", "owner"); err != nil || !ok {
		t.Fatalf("remove: %v %v", ok, err)
	}
	rows, err = s.UsersBrief(ctx, "owner", ids)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(ids8(rows)); got != "[f_in]" {
		t.Fatalf("resolved %s after unfriending", got)
	}

	for _, empty := range [][]string{nil, {}} {
		rows, err := s.UsersBrief(ctx, "owner", empty)
		if err != nil || rows == nil || len(rows) != 0 {
			t.Fatalf("empty list: %v %v", rows, err)
		}
		friends, err := s.AcceptedFriendIDs(ctx, "owner", empty)
		if err != nil || len(friends) != 0 {
			t.Fatalf("empty list: %v %v", friends, err)
		}
	}
}

// BirthMoment against the platform's real column types: the offset column is
// an INTEGER, so fractional zones only ever arrive as legacy minutes.
func TestBirthMomentReadsLegacyMinuteOffsets(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "india", "whole", "notime", "nodata")
	for _, q := range []string{
		`UPDATE users SET birth_date = '1994-03-21', birth_time = '10:30', birth_timezone_offset = -330,
		        birth_latitude = 28.6139, birth_longitude = 77.2090 WHERE id = 'india'`,
		`UPDATE users SET birth_date = '1994-03-21', birth_time = '10:30', birth_timezone_offset = 1,
		        birth_latitude = 52.2297, birth_longitude = 21.0122 WHERE id = 'whole'`,
		`UPDATE users SET birth_date = '1994-03-21', birth_timezone_offset = -345 WHERE id = 'notime'`,
	} {
		if _, err := fx.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	for id, want := range map[string]string{
		"india":  "1994-03-21 05:00",
		"whole":  "1994-03-21 09:30",
		"notime": "1994-03-21 06:15", // local noon at +5.75
	} {
		bm, err := s.BirthMoment(ctx, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got := bm.UTC.Format("2006-01-02 15:04"); got != want {
			t.Fatalf("%s: %s, want %s", id, got, want)
		}
	}
	bm, err := s.BirthMoment(ctx, "india")
	if err != nil || bm.Latitude != 28.6139 || bm.Longitude != 77.209 {
		t.Fatalf("place lost: %+v %v", bm, err)
	}
	for _, id := range []string{"nodata", "no-such-account"} {
		if _, err := s.BirthMoment(ctx, id); err == nil {
			t.Fatalf("%s: birth data invented", id)
		}
	}
}

// The wait after a decline cannot be skipped by deleting the declined request:
// the member who was declined gets "done" and the row stays; the member who
// declined can clear it, after which either may ask.
func TestADeclinedRequestIsOnlyClearedByTheMemberWhoDeclined(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "bob")
	if got, err := s.CreateFriendRequest(ctx, "ann", "bob"); err != nil || got != "created" {
		t.Fatalf("request: %q, %v", got, err)
	}
	var id string
	if err := fx.QueryRow(ctx, `SELECT id FROM friendships WHERE requester_id = 'ann'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.RespondFriendRequest(ctx, id, "bob", false); err != nil || !ok {
		t.Fatalf("decline: %v %v", ok, err)
	}
	// Ann withdraws: it looks done to her, but the declined row is kept...
	if ok, err := s.RemoveFriend(ctx, "ann", "bob"); err != nil || !ok {
		t.Fatalf("ann withdraws: %v %v", ok, err)
	}
	if n := edgeCount(t, fx, "ann", "bob"); n != 1 {
		t.Fatalf("%d rows after the declined member withdrew, want 1", n)
	}
	// ...so asking again is still held back.
	if got, err := s.CreateFriendRequest(ctx, "ann", "bob"); err != nil || got != "exists" {
		t.Fatalf("ann asks again: %q, %v", got, err)
	}
	// It is still a "no" underneath: marked as withdrawn while she had it
	// withdrawn, and declined again now that she has sent it again.
	var status string
	if err := fx.QueryRow(ctx, `SELECT status FROM friendships WHERE requester_id = 'ann' AND addressee_id = 'bob'`).Scan(&status); err != nil || status != "declined" {
		t.Fatalf("status %q, %v", status, err)
	}
	// Bob, who declined, can clear it; then the pair starts afresh.
	if ok, err := s.RemoveFriend(ctx, "bob", "ann"); err != nil || !ok {
		t.Fatalf("bob clears: %v %v", ok, err)
	}
	if n := edgeCount(t, fx, "ann", "bob"); n != 0 {
		t.Fatalf("%d rows after the decliner cleared it", n)
	}
	if got, err := s.CreateFriendRequest(ctx, "ann", "bob"); err != nil || got != "created" {
		t.Fatalf("ann asks after bob cleared: %q, %v", got, err)
	}
}

// UserByNick tells "nobody goes by that" apart from "the lookup failed": the
// first is answered to the member as a request that was sent, the second
// must not be.
func TestUserByNickSeparatesNotFoundFromFailure(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "chosen", "user_1_00000000ab12")
	if _, err := fx.Exec(ctx, `UPDATE users SET nick = 'Luna.Moon' WHERE id = 'chosen'`); err != nil {
		t.Fatal(err)
	}
	for handle, want := range map[string]string{
		"Luna.Moon": "chosen", " luna.moon ": "chosen",
		"seeker-ab12": "user_1_00000000ab12", "Seeker-AB12": "user_1_00000000ab12",
	} {
		u, err := s.UserByNick(ctx, handle)
		if err != nil || u == nil || u.ID != want {
			t.Fatalf("%q: %+v, %v (want %s)", handle, u, err, want)
		}
	}
	for _, handle := range []string{"nobody", "seeker-ffff", "seeker-ab123", "seeker-", ""} {
		if u, err := s.UserByNick(ctx, handle); !errors.Is(err, ErrNoSuchUser) || u != nil {
			t.Fatalf("%q: %+v, %v (want ErrNoSuchUser)", handle, u, err)
		}
	}
	// Two members without a nick whose ids end alike: the handle is nobody's.
	// A third who chose a nick does not count; when one of the two chooses
	// one, the handle belongs to the other again.
	addUsers(t, fx, "user_2_11111111AB12", "user_3_22222222ab12")
	if _, err := fx.Exec(ctx, `UPDATE users SET nick = 'Third' WHERE id = 'user_3_22222222ab12'`); err != nil {
		t.Fatal(err)
	}
	for _, handle := range []string{"seeker-ab12", "SEEKER-AB12"} {
		if u, err := s.UserByNick(ctx, handle); !errors.Is(err, ErrNoSuchUser) || u != nil {
			t.Fatalf("shared handle %q: %+v, %v (want ErrNoSuchUser)", handle, u, err)
		}
	}
	if _, err := fx.Exec(ctx, `UPDATE users SET nick = 'First' WHERE id = 'user_1_00000000ab12'`); err != nil {
		t.Fatal(err)
	}
	if u, err := s.UserByNick(ctx, "seeker-ab12"); err != nil || u == nil || u.ID != "user_2_11111111AB12" {
		t.Fatalf("handle with one member left: %+v, %v", u, err)
	}

	// A request that is already over when the lookup runs: an error, and not
	// the "no such member" one.
	gone, cancel := context.WithCancel(ctx)
	cancel()
	for _, handle := range []string{"Luna.Moon", "nobody", "seeker-ab12"} {
		if u, err := s.UserByNick(gone, handle); err == nil || errors.Is(err, ErrNoSuchUser) || u != nil {
			t.Fatalf("%q on a dead request: %+v, %v", handle, u, err)
		}
	}
}

// bulkUsers adds n accounts named prefix0001.. with the given picture.
func bulkUsers(t *testing.T, pool *pgxpool.Pool, prefix string, n int, avatar []byte) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, nick, avatar, avatar_mime)
		 SELECT $1::text || to_char(g, 'FM0000'), $1::text || g || '@example.test', $1::text || '.' || g, $3::bytea, 'image/png'
		 FROM generate_series(1, $2::int) g`, prefix, n, avatar); err != nil {
		t.Fatalf("bulk users %s: %v", prefix, err)
	}
}

// bulkEdges links prefix0001.. to member. The n-th edge is n minutes newer
// than the first, so "newest" is the highest number.
func bulkEdges(t *testing.T, pool *pgxpool.Pool, prefix string, n int, member, status string, memberAsks bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO friendships (id, requester_id, addressee_id, status, created_at, responded_at)
		 SELECT 'fr_' || $1::text || to_char(g, 'FM0000'),
		        CASE WHEN $5::bool THEN $3::text ELSE $1::text || to_char(g, 'FM0000') END,
		        CASE WHEN $5::bool THEN $1::text || to_char(g, 'FM0000') ELSE $3::text END,
		        $4::text, NOW() - INTERVAL '30 days' + make_interval(mins => g),
		        CASE WHEN $4::text = 'pending' THEN NULL ELSE NOW() END
		 FROM generate_series(1, $2::int) g`, prefix, n, member, status, memberAsks); err != nil {
		t.Fatalf("bulk edges %s: %v", prefix, err)
	}
}

// The friends list is one response, and its size is bounded. Unbounded, with
// a picture on every row, it could be pushed past any bound by
// anyone willing to register accounts and send requests, and then the member
// could not even open the list to decline them.
func TestFriendListIsBoundedAndPicturesStayWithinABudget(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "me")
	full := make([]byte, MaxAvatarBytes) // the largest picture a member may store

	bulkUsers(t, fx, "pal", maxFriendsListed+20, full)
	bulkEdges(t, fx, "pal", maxFriendsListed+20, "me", "accepted", false)
	bulkUsers(t, fx, "asker", maxPendingListed+30, full)
	bulkEdges(t, fx, "asker", maxPendingListed+30, "me", "pending", false)
	bulkUsers(t, fx, "asked", maxPendingListed+30, full)
	bulkEdges(t, fx, "asked", maxPendingListed+30, "me", "pending", true)
	// A picture stored before the size cap existed: never sent, and it does
	// not use up the budget either.
	if _, err := fx.Exec(ctx, `UPDATE users SET avatar = $1 WHERE id = 'pal0003'`, make([]byte, MaxAvatarBytes+1)); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListFriendships(ctx, "me")
	if err != nil {
		t.Fatal(err)
	}

	// Accepted: at most maxFriendsListed, every one named, pictures only for
	// as many as the budget holds.
	if len(got.Accepted) != maxFriendsListed {
		t.Fatalf("%d friends listed, want %d", len(got.Accepted), maxFriendsListed)
	}
	if got.Accepted[0].UserID != "pal0001" || got.Accepted[maxFriendsListed-1].UserID != fmt.Sprintf("pal%04d", maxFriendsListed) {
		t.Fatalf("friends are not in the order the friendships began: first %s, last %s",
			got.Accepted[0].UserID, got.Accepted[maxFriendsListed-1].UserID)
	}
	withPicture, pictureBytes := 0, 0
	for i, friend := range got.Accepted {
		if friend.UserID == "" || friend.Nick == "" || friend.Since == "" || friend.Synastry != "none" {
			t.Fatalf("friend %d lost a field: %+v", i, friend)
		}
		if friend.Avatar == "" {
			continue
		}
		if friend.UserID == "pal0003" {
			t.Fatal("a picture over the size cap was sent")
		}
		withPicture++
		pictureBytes += MaxAvatarBytes
		if i > friendListAvatarBudget/MaxAvatarBytes {
			t.Fatalf("friend %d has a picture although earlier friends went without", i)
		}
	}
	if want := friendListAvatarBudget / MaxAvatarBytes; withPicture != want {
		t.Fatalf("%d friends carry a picture, want exactly the %d the budget holds", withPicture, want)
	}
	if pictureBytes > friendListAvatarBudget {
		t.Fatalf("%d bytes of pictures, over the budget of %d", pictureBytes, friendListAvatarBudget)
	}

	// Requests: the newest maxPendingListed of each, newest first.
	if len(got.Incoming) != maxPendingListed || len(got.Outgoing) != maxPendingListed {
		t.Fatalf("%d incoming and %d outgoing listed, want %d of each", len(got.Incoming), len(got.Outgoing), maxPendingListed)
	}
	for i := 0; i < maxPendingListed; i++ {
		n := maxPendingListed + 30 - i
		if in, out := got.Incoming[i], got.Outgoing[i]; in.UserID != fmt.Sprintf("asker%04d", n) || out.UserID != fmt.Sprintf("asked%04d", n) ||
			in.RequestID != "fr_"+in.UserID || out.RequestID != "fr_"+out.UserID || in.Nick == "" || out.Nick == "" {
			t.Fatalf("request %d is not the %d-th: incoming %+v, outgoing %+v", i, n, in, out)
		}
	}

	// The whole reply, as the handler writes it, well within its bound. No
	// request carries a picture.
	body, err := json.Marshal(map[string]any{"success": true, "friends": got})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > 3<<20 {
		t.Fatalf("the friends reply is %d bytes", len(body))
	}
	requests, _ := json.Marshal(map[string]any{"incoming": got.Incoming, "outgoing": got.Outgoing})
	if strings.Contains(string(requests), "avatar") || strings.Contains(string(requests), "base64") {
		t.Fatal("a pending request carries a picture")
	}
}

// outgoingOf is what a member sees under "requests I sent".
func outgoingOf(t *testing.T, s *Store, member string) []FriendRequestEntry {
	t.Helper()
	list, err := s.ListFriendships(context.Background(), member)
	if err != nil {
		t.Fatal(err)
	}
	return list.Outgoing
}

// To the member whose request was declined, it goes on looking like a request
// that is waiting: listed while they leave it, gone when they withdraw it,
// back when they send it again. Every step is done twice here, once on a
// request that really is waiting (to cyd) and once on a declined one (to bob),
// and the two must be indistinguishable.
func TestADeclinedFriendRequestLooksLikeOneStillWaiting(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "bob", "cyd")
	row := func(a, b string) (id, requester, status string, responded *time.Time) {
		t.Helper()
		if err := fx.QueryRow(ctx,
			`SELECT id, requester_id, status, responded_at FROM friendships
			 WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)`,
			a, b).Scan(&id, &requester, &status, &responded); err != nil {
			t.Fatal(err)
		}
		return
	}
	entryFor := func(list []FriendRequestEntry, other string) *FriendRequestEntry {
		for i := range list {
			if list[i].UserID == other {
				return &list[i]
			}
		}
		return nil
	}

	for _, to := range []string{"bob", "cyd"} {
		if got, err := s.CreateFriendRequest(ctx, "ann", to); err != nil || got != "created" {
			t.Fatalf("ann→%s: %q, %v", to, got, err)
		}
	}
	before := outgoingOf(t, s, "ann")
	if len(before) != 2 {
		t.Fatalf("ann's sent requests: %+v", before)
	}
	bobID, _, _, _ := row("ann", "bob")
	if ok, err := s.RespondFriendRequest(ctx, bobID, "bob", false); err != nil || !ok {
		t.Fatalf("bob declines: %v %v", ok, err)
	}
	_, _, _, declinedAt := row("ann", "bob")

	// Declined: nothing ann can read has changed.
	if after := outgoingOf(t, s, "ann"); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("the decline shows in ann's list:\nbefore %+v\nafter  %+v", before, after)
	}
	// Bob, who declined, sees no request from ann in any direction.
	if list, err := s.ListFriendships(ctx, "bob"); err != nil || len(list.Incoming)+len(list.Outgoing)+len(list.Accepted) != 0 {
		t.Fatalf("bob's list after declining: %+v %v", list, err)
	}

	// Ann withdraws both. Both report done, both are gone from her list, and
	// a second withdrawal finds nothing in both cases.
	for _, to := range []string{"bob", "cyd"} {
		if ok, err := s.RemoveFriend(ctx, "ann", to); err != nil || !ok {
			t.Fatalf("ann withdraws the request to %s: %v %v", to, ok, err)
		}
	}
	if left := outgoingOf(t, s, "ann"); len(left) != 0 {
		t.Fatalf("withdrawn requests are still listed: %+v", left)
	}
	for _, to := range []string{"bob", "cyd"} {
		if ok, err := s.RemoveFriend(ctx, "ann", to); err != nil || ok {
			t.Fatalf("ann withdraws the request to %s a second time: %v %v", to, ok, err)
		}
	}
	// Underneath, the declined one is kept and its clock has not moved.
	if _, requester, status, at := row("ann", "bob"); requester != "ann" || status != "withdrawn" || at == nil || !at.Equal(*declinedAt) {
		t.Fatalf("declined request after ann withdrew it: %s %s %v (declined at %v)", requester, status, at, declinedAt)
	}
	if n := edgeCount(t, fx, "ann", "cyd"); n != 0 {
		t.Fatalf("%d rows left of a waiting request that was withdrawn", n)
	}

	// Ann sends both again. Both are listed again, as new requests: newest
	// first, each under an id it did not have before.
	for _, to := range []string{"cyd", "bob"} {
		if _, err := s.CreateFriendRequest(ctx, "ann", to); err != nil {
			t.Fatalf("ann→%s again: %v", to, err)
		}
		time.Sleep(2 * time.Millisecond) // ids begin with the millisecond
	}
	again := outgoingOf(t, s, "ann")
	if len(again) != 2 || again[0].UserID != "bob" || again[1].UserID != "cyd" {
		t.Fatalf("ann's list after sending both again: %+v", again)
	}
	for _, e := range again {
		if old := entryFor(before, e.UserID); old == nil || old.RequestID == e.RequestID {
			t.Fatalf("the request to %s came back under its old id %s", e.UserID, e.RequestID)
		}
	}
	// Cyd got a new request. Bob got nothing, and his "no" still stands from
	// the moment he gave it.
	if list, err := s.ListFriendships(ctx, "cyd"); err != nil || len(list.Incoming) != 1 {
		t.Fatalf("cyd's list: %+v %v", list, err)
	}
	if list, err := s.ListFriendships(ctx, "bob"); err != nil || len(list.Incoming)+len(list.Outgoing) != 0 {
		t.Fatalf("bob was reached inside the wait: %+v %v", list, err)
	}
	if _, requester, status, at := row("ann", "bob"); requester != "ann" || status != "declined" || at == nil || !at.Equal(*declinedAt) {
		t.Fatalf("declined request after ann sent it again: %s %s %v (declined at %v)", requester, status, at, declinedAt)
	}

	// Once the wait is over, asking again reaches bob. Ann's list does not
	// change by a character: same entries, same ids, same order.
	listed := outgoingOf(t, s, "ann")
	if _, err := fx.Exec(ctx, `UPDATE friendships SET responded_at = NOW() - INTERVAL '8 days' WHERE requester_id = 'ann' AND addressee_id = 'bob'`); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CreateFriendRequest(ctx, "ann", "bob"); err != nil || got != "created" {
		t.Fatalf("ann→bob after the wait: %q, %v", got, err)
	}
	if now := outgoingOf(t, s, "ann"); fmt.Sprint(now) != fmt.Sprint(listed) {
		t.Fatalf("reopening after the wait shows in ann's list:\nbefore %+v\nafter  %+v", listed, now)
	}
	if list, err := s.ListFriendships(ctx, "bob"); err != nil || len(list.Incoming) != 1 || list.Incoming[0].UserID != "ann" {
		t.Fatalf("bob's list after the wait: %+v %v", list, err)
	}
}

// A declined request its sender withdrew is still a "no" with its wait: it can
// be reopened by the member who declined at any time, by the sender only after
// the wait, and cleared by the member who declined.
func TestAWithdrawnDeclinedRequestKeepsItsRules(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	status := func(a, b string) string {
		t.Helper()
		var requester, st string
		err := fx.QueryRow(ctx,
			`SELECT requester_id, status FROM friendships
			 WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)`, a, b).Scan(&requester, &st)
		if errors.Is(err, pgx.ErrNoRows) {
			return "none"
		}
		if err != nil {
			t.Fatal(err)
		}
		return requester + ":" + st
	}
	// declinedAndWithdrawn leaves a→b declined by b and then withdrawn by a.
	declinedAndWithdrawn := func(a, b string) {
		t.Helper()
		addUsers(t, fx, a, b)
		if got, err := s.CreateFriendRequest(ctx, a, b); err != nil || got != "created" {
			t.Fatalf("%s→%s: %q, %v", a, b, got, err)
		}
		var id string
		if err := fx.QueryRow(ctx, `SELECT id FROM friendships WHERE requester_id = $1 AND addressee_id = $2`, a, b).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if ok, err := s.RespondFriendRequest(ctx, id, b, false); err != nil || !ok {
			t.Fatalf("%s declines: %v %v", b, ok, err)
		}
		if ok, err := s.RemoveFriend(ctx, a, b); err != nil || !ok {
			t.Fatalf("%s withdraws: %v %v", a, ok, err)
		}
		if got := status(a, b); got != a+":withdrawn" {
			t.Fatalf("%s→%s is %s", a, b, got)
		}
	}

	// The member who declined changes their mind: a request from them.
	declinedAndWithdrawn("a1", "b1")
	if got, err := s.CreateFriendRequest(ctx, "b1", "a1"); err != nil || got != "created" || status("a1", "b1") != "b1:pending" {
		t.Fatalf("the member who declined asks: %q, %v, %s", got, err, status("a1", "b1"))
	}
	if out := outgoingOf(t, s, "b1"); len(out) != 1 || out[0].UserID != "a1" {
		t.Fatalf("b1's sent requests: %+v", out)
	}

	// The sender asks after the wait: a real request again.
	declinedAndWithdrawn("a2", "b2")
	if _, err := fx.Exec(ctx, `UPDATE friendships SET responded_at = NOW() - INTERVAL '8 days' WHERE requester_id = 'a2'`); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CreateFriendRequest(ctx, "a2", "b2"); err != nil || got != "created" || status("a2", "b2") != "a2:pending" {
		t.Fatalf("the sender asks after the wait: %q, %v, %s", got, err, status("a2", "b2"))
	}

	// The member who declined clears it: the pair starts afresh.
	declinedAndWithdrawn("a3", "b3")
	if ok, err := s.RemoveFriend(ctx, "b3", "a3"); err != nil || !ok || status("a3", "b3") != "none" {
		t.Fatalf("the member who declined clears it: %v %v %s", ok, err, status("a3", "b3"))
	}
	if got, err := s.CreateFriendRequest(ctx, "a3", "b3"); err != nil || got != "created" {
		t.Fatalf("after it was cleared: %q, %v", got, err)
	}

	// Withdrawing and sending again, over and over, never reaches the member
	// who declined and never shortens the wait.
	declinedAndWithdrawn("a4", "b4")
	for i := 0; i < 3; i++ {
		if got, err := s.CreateFriendRequest(ctx, "a4", "b4"); err != nil || got != "exists" || status("a4", "b4") != "a4:declined" {
			t.Fatalf("round %d, send again: %q, %v, %s", i, got, err, status("a4", "b4"))
		}
		if ok, err := s.RemoveFriend(ctx, "a4", "b4"); err != nil || !ok || status("a4", "b4") != "a4:withdrawn" {
			t.Fatalf("round %d, withdraw: %v, %v, %s", i, ok, err, status("a4", "b4"))
		}
	}
	if list, err := s.ListFriendships(ctx, "b4"); err != nil || len(list.Incoming) != 0 {
		t.Fatalf("b4 was reached: %+v %v", list, err)
	}
}

func sentCount(t *testing.T, pool *pgxpool.Pool, member, status string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM friendships WHERE requester_id = $1 AND status = $2`, member, status).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// One member may have MaxSentFriendRequests requests waiting at a time. Past
// that a request is answered like one that already exists, and nothing is
// stored. A request that was declined still counts: the member is shown it
// as waiting, and a limit that ignored it would give the decline away.
func TestSentFriendRequestsAreCapped(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "sender")
	bulkUsers(t, fx, "to", MaxSentFriendRequests+30, nil)
	to := func(n int) string { return fmt.Sprintf("to%04d", n) }
	send := func(n int) string {
		t.Helper()
		got, err := s.CreateFriendRequest(ctx, "sender", to(n))
		if err != nil {
			t.Fatalf("request %d: %v", n, err)
		}
		return got
	}
	answer := func(n int, accept bool) {
		t.Helper()
		var id string
		if err := fx.QueryRow(ctx, `SELECT id FROM friendships WHERE requester_id = 'sender' AND addressee_id = $1`, to(n)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if ok, err := s.RespondFriendRequest(ctx, id, to(n), accept); err != nil || !ok {
			t.Fatalf("answer %d: %v %v", n, ok, err)
		}
	}

	for n := 1; n <= MaxSentFriendRequests; n++ {
		if got := send(n); got != "created" {
			t.Fatalf("request %d of %d: %q", n, MaxSentFriendRequests, got)
		}
	}
	over := MaxSentFriendRequests + 1
	if got := send(over); got != "exists" || edgeCount(t, fx, "sender", to(over)) != 0 {
		t.Fatalf("request %d: %q, %d rows — over the limit, nothing may be stored", over, got, edgeCount(t, fx, "sender", to(over)))
	}
	if n := len(outgoingOf(t, s, "sender")); n != MaxSentFriendRequests {
		t.Fatalf("%d requests listed as sent, want %d", n, MaxSentFriendRequests)
	}

	// A decline frees nothing: the sender still has that request on their
	// list, and must not learn of the decline from a request going through.
	answer(1, false)
	if got := send(over); got != "exists" || edgeCount(t, fx, "sender", to(over)) != 0 {
		t.Fatalf("after one request was declined: %q", got)
	}
	// Each of these takes one request off the sender's list, and so makes
	// room for one more: an acceptance, withdrawing a waiting request, and
	// withdrawing the declined one.
	answer(2, true)
	if got := send(over); got != "created" {
		t.Fatalf("after one request was accepted: %q", got)
	}
	if ok, err := s.RemoveFriend(ctx, "sender", to(3)); err != nil || !ok {
		t.Fatalf("withdraw a waiting request: %v %v", ok, err)
	}
	if got := send(over + 1); got != "created" {
		t.Fatalf("after a waiting request was withdrawn: %q", got)
	}
	if ok, err := s.RemoveFriend(ctx, "sender", to(1)); err != nil || !ok {
		t.Fatalf("withdraw the declined request: %v %v", ok, err)
	}
	if got := send(over + 2); got != "created" {
		t.Fatalf("after the declined request was withdrawn: %q", got)
	}
	// Full again. The withdrawn one cannot be put back on the list either.
	if got := send(1); got != "exists" || sentCount(t, fx, "sender", "withdrawn") != 1 {
		t.Fatalf("sending the withdrawn request again while full: %q", got)
	}
	if got := send(over + 3); got != "exists" || edgeCount(t, fx, "sender", to(over+3)) != 0 {
		t.Fatalf("full again: %q", got)
	}
	if n := len(outgoingOf(t, s, "sender")); n != MaxSentFriendRequests {
		t.Fatalf("%d requests listed as sent, want %d", n, MaxSentFriendRequests)
	}

	// The limit is on requests sent. Receiving them, and answering a decline
	// of one's own with a request, is a matter for the other member's limit.
	if got, err := s.CreateFriendRequest(ctx, to(over+4), "sender"); err != nil || got != "created" {
		t.Fatalf("a request TO a member at the limit: %q, %v", got, err)
	}

	// Many at once from a member five short of the limit: exactly five go
	// through.
	for n := 10; n < 15; n++ {
		if ok, err := s.RemoveFriend(ctx, "sender", to(n)); err != nil || !ok {
			t.Fatalf("withdraw %d: %v %v", n, ok, err)
		}
	}
	const callers = 20
	results := make([]string, callers)
	errs := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = s.CreateFriendRequest(ctx, "sender", to(over+5+i))
		}(i)
	}
	close(start)
	wg.Wait()
	created := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if results[i] == "created" {
			created++
		}
	}
	if created != 5 || len(outgoingOf(t, s, "sender")) != MaxSentFriendRequests {
		t.Fatalf("%d of %d simultaneous requests went through, %d listed; want 5 and %d",
			created, callers, len(outgoingOf(t, s, "sender")), MaxSentFriendRequests)
	}
}

// The member who declined answers with a request of their own: it counts
// against THEIR limit, as any request they send does.
func TestReopeningADeclineCountsAgainstTheOneWhoAsks(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "busy", "asker")
	bulkUsers(t, fx, "to", MaxSentFriendRequests, nil)
	// asker → busy, declined by busy.
	if got, err := s.CreateFriendRequest(ctx, "asker", "busy"); err != nil || got != "created" {
		t.Fatalf("request: %q, %v", got, err)
	}
	var id string
	if err := fx.QueryRow(ctx, `SELECT id FROM friendships WHERE requester_id = 'asker'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.RespondFriendRequest(ctx, id, "busy", false); err != nil || !ok {
		t.Fatalf("decline: %v %v", ok, err)
	}
	// busy fills their own list.
	bulkEdges(t, fx, "to", MaxSentFriendRequests, "busy", "pending", true)
	if got, err := s.CreateFriendRequest(ctx, "busy", "asker"); err != nil || got != "exists" {
		t.Fatalf("the member who declined asks while at the limit: %q, %v", got, err)
	}
	if n := sentCount(t, fx, "asker", "declined"); n != 1 {
		t.Fatalf("the declined request changed although nothing was sent (%d declined rows)", n)
	}
	if _, err := fx.Exec(ctx, `DELETE FROM friendships WHERE requester_id = 'busy' AND addressee_id = 'to0001'`); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CreateFriendRequest(ctx, "busy", "asker"); err != nil || got != "created" {
		t.Fatalf("the member who declined asks with room: %q, %v", got, err)
	}
}

// consentRow reads the pair's consent row as stored; status "none" when
// there is none.
func consentRow(t *testing.T, pool *pgxpool.Pool, a, b string) (status, requestedBy string, responded *time.Time) {
	t.Helper()
	lo, hi := pairLoHi(a, b)
	err := pool.QueryRow(context.Background(),
		`SELECT status, requested_by, responded_at FROM synastry_consents WHERE user_lo = $1 AND user_hi = $2`,
		lo, hi).Scan(&status, &requestedBy, &responded)
	if errors.Is(err, pgx.ErrNoRows) {
		return "none", "", nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return status, requestedBy, responded
}

// synastryAs is the consent state member is shown for their friend other.
func synastryAs(t *testing.T, s *Store, member, other string) string {
	t.Helper()
	list, err := s.ListFriendships(context.Background(), member)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range list.Accepted {
		if f.UserID == other {
			return f.Synastry
		}
	}
	t.Fatalf("%s is not listed as a friend of %s", other, member)
	return ""
}

// A declined chart-comparison request follows the rule of a declined friend
// request: the member who declined may ask in their own turn at any time, the
// member who was declined only a week after the decline. It used to be
// reopened by simply asking again.
func TestADeclinedSynastryRequestWaitsAWeek(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "bob")
	befriend(t, fx, "ann", "bob", "accepted")
	request := func(me, other, want string) {
		t.Helper()
		if got, err := s.RequestSynastry(ctx, me, other); err != nil || got != want {
			t.Fatalf("%s asks %s: %q, %v (want %q)", me, other, got, err, want)
		}
	}
	age := func(interval string) {
		t.Helper()
		if _, err := fx.Exec(ctx, `UPDATE synastry_consents SET responded_at = NOW() - $1::interval`, interval); err != nil {
			t.Fatal(err)
		}
	}
	unread := func(member string) int {
		t.Helper()
		u, err := s.UnreadCounts(ctx, member)
		if err != nil {
			t.Fatal(err)
		}
		return u.SynastryRequests
	}

	request("ann", "bob", "requested")
	if ok, err := s.RespondSynastry(ctx, "bob", "ann", false); err != nil || !ok {
		t.Fatalf("bob declines: %v %v", ok, err)
	}
	status, by, declinedAt := consentRow(t, fx, "ann", "bob")
	if status != "declined" || by != "ann" || declinedAt == nil {
		t.Fatalf("after the decline: %s by %s, answered at %v — the time of the decline must be recorded", status, by, declinedAt)
	}

	// Ann asks again at once, and just inside the week: answered like a
	// request that is still waiting, and nothing changes or reaches Bob.
	for _, when := range []string{"0 seconds", "6 days 23 hours"} {
		age(when)
		request("ann", "bob", "already-requested")
		if status, by, at := consentRow(t, fx, "ann", "bob"); status != "declined" || by != "ann" || at == nil {
			t.Fatalf("ann asked again %s after the decline: %s by %s, answered at %v", when, status, by, at)
		}
		if n := unread("bob"); n != 0 {
			t.Fatalf("bob was asked again %s after declining", when)
		}
	}
	// After the week it is a request to Bob again. Ann is told what she was
	// told all along.
	age("7 days 1 minute")
	request("ann", "bob", "already-requested")
	if status, by, at := consentRow(t, fx, "ann", "bob"); status != "pending" || by != "ann" || at != nil {
		t.Fatalf("ann asked after the week: %s by %s, answered at %v", status, by, at)
	}
	if n := unread("bob"); n != 1 {
		t.Fatalf("bob has %d new requests after the week, want 1", n)
	}

	// Bob declines again and then changes his mind: no wait for the one who
	// declined, and the request is now his.
	if ok, err := s.RespondSynastry(ctx, "bob", "ann", false); err != nil || !ok {
		t.Fatalf("bob declines again: %v %v", ok, err)
	}
	request("bob", "ann", "requested")
	if status, by, at := consentRow(t, fx, "ann", "bob"); status != "pending" || by != "bob" || at != nil {
		t.Fatalf("bob asked after declining: %s by %s, answered at %v", status, by, at)
	}
	if got := synastryAs(t, s, "ann", "bob"); got != "pending-in" {
		t.Fatalf("ann sees %q, want a request from bob", got)
	}
	// Ann, who was declined a moment ago, may accept it by asking back.
	request("ann", "bob", "accepted")
	request("ann", "bob", "already-accepted")
}

// To the member whose chart-comparison request was declined it goes on looking
// like one that is waiting, through every step they can take. Each step is
// taken on a waiting request (ann→cyd) and on a declined one (ann→bob).
func TestADeclinedSynastryRequestLooksLikeOneStillWaiting(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "bob", "cyd")
	befriend(t, fx, "ann", "bob", "accepted")
	befriend(t, fx, "cyd", "ann", "accepted")
	both := func(step string, do func(other string) string) {
		t.Helper()
		declined, waiting := do("bob"), do("cyd")
		if declined != waiting {
			t.Fatalf("%s: %q for the declined request, %q for the waiting one", step, declined, waiting)
		}
	}
	ask := func(other string) string {
		got, err := s.RequestSynastry(ctx, "ann", other)
		if err != nil {
			t.Fatal(err)
		}
		return got + ", shown as " + synastryAs(t, s, "ann", other)
	}
	revoke := func(other string) string {
		ok, err := s.RevokeSynastry(ctx, "ann", other)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(ok) + ", shown as " + synastryAs(t, s, "ann", other)
	}

	both("first request", ask)
	if ok, err := s.RespondSynastry(ctx, "bob", "ann", false); err != nil || !ok {
		t.Fatalf("bob declines: %v %v", ok, err)
	}
	_, _, declinedAt := consentRow(t, fx, "ann", "bob")

	both("as listed", func(other string) string { return synastryAs(t, s, "ann", other) })
	if got := synastryAs(t, s, "ann", "bob"); got != "pending-out" {
		t.Fatalf("ann sees %q for the request bob declined, want pending-out", got)
	}
	if got := synastryAs(t, s, "bob", "ann"); got != "declined" {
		t.Fatalf("bob sees %q for the request he declined", got)
	}
	both("asking again", ask)
	both("cancelling", revoke)
	if got := synastryAs(t, s, "ann", "bob"); got != "none" {
		t.Fatalf("ann sees %q after cancelling", got)
	}
	both("cancelling again", revoke)
	// Underneath, Bob's "no" is still there with its clock, and he still sees
	// it as declined.
	if status, by, at := consentRow(t, fx, "ann", "bob"); status != "withdrawn" || by != "ann" || at == nil || !at.Equal(*declinedAt) {
		t.Fatalf("after ann cancelled: %s by %s, answered at %v (declined at %v)", status, by, at, declinedAt)
	}
	if got := synastryAs(t, s, "bob", "ann"); got != "declined" {
		t.Fatalf("bob sees %q after ann cancelled", got)
	}
	if status, _, _ := consentRow(t, fx, "ann", "cyd"); status != "none" {
		t.Fatalf("the waiting request ann cancelled is still stored as %s", status)
	}

	both("asking after cancelling", ask)
	// Cyd was asked again; Bob was not, and his clock has not moved.
	if status, by, at := consentRow(t, fx, "ann", "bob"); status != "declined" || by != "ann" || at == nil || !at.Equal(*declinedAt) {
		t.Fatalf("after ann asked again: %s by %s, answered at %v (declined at %v)", status, by, at, declinedAt)
	}
	for member, want := range map[string]int{"bob": 0, "cyd": 1} {
		if u, err := s.UnreadCounts(ctx, member); err != nil || u.SynastryRequests != want {
			t.Fatalf("%s has %d new requests, want %d (%v)", member, u.SynastryRequests, want, err)
		}
	}

	// Cancelling and asking again, however often, never reaches Bob.
	for i := 0; i < 3; i++ {
		both("cancelling, round "+fmt.Sprint(i), revoke)
		both("asking, round "+fmt.Sprint(i), ask)
	}
	if status, by, at := consentRow(t, fx, "ann", "bob"); status != "declined" || by != "ann" || !at.Equal(*declinedAt) {
		t.Fatalf("after three rounds: %s by %s, answered at %v", status, by, at)
	}

	// Cancelled, and then the week passes: asking is a real request again.
	both("cancelling before the wait ends", revoke)
	if _, err := fx.Exec(ctx, `UPDATE synastry_consents SET responded_at = NOW() - INTERVAL '8 days' WHERE requested_by = 'ann' AND status = 'withdrawn'`); err != nil {
		t.Fatal(err)
	}
	both("asking after the wait", ask)
	if status, by, at := consentRow(t, fx, "ann", "bob"); status != "pending" || by != "ann" || at != nil {
		t.Fatalf("after the wait: %s by %s, answered at %v", status, by, at)
	}
}

// The member who declined keeps every option they had: ask in their own turn,
// or clear the request, whether or not its sender has cancelled it since.
func TestTheMemberWhoDeclinedSynastryCanStillReopenOrClearIt(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		cancelled bool // ann cancels after bob declined
		clear     bool // bob clears instead of asking
	}{
		{"asks", false, false}, {"asks after it was cancelled", true, false},
		{"clears", false, true}, {"clears after it was cancelled", true, true},
	} {
		a, b := "ann_"+fmt.Sprint(tc.cancelled, tc.clear), "bob_"+fmt.Sprint(tc.cancelled, tc.clear)
		addUsers(t, fx, a, b)
		befriend(t, fx, a, b, "accepted")
		if got, err := s.RequestSynastry(ctx, a, b); err != nil || got != "requested" {
			t.Fatalf("%s: request: %q, %v", tc.name, got, err)
		}
		if ok, err := s.RespondSynastry(ctx, b, a, false); err != nil || !ok {
			t.Fatalf("%s: decline: %v %v", tc.name, ok, err)
		}
		if tc.cancelled {
			if ok, err := s.RevokeSynastry(ctx, a, b); err != nil || !ok {
				t.Fatalf("%s: cancel: %v %v", tc.name, ok, err)
			}
		}
		if tc.clear {
			if ok, err := s.RevokeSynastry(ctx, b, a); err != nil || !ok {
				t.Fatalf("%s: clear: %v %v", tc.name, ok, err)
			}
			if status, _, _ := consentRow(t, fx, a, b); status != "none" {
				t.Fatalf("%s: row is %s after the member who declined cleared it", tc.name, status)
			}
			// Cleared by the one who said no: the other may ask at once.
			if got, err := s.RequestSynastry(ctx, a, b); err != nil || got != "requested" {
				t.Fatalf("%s: request after clearing: %q, %v", tc.name, got, err)
			}
			continue
		}
		if got, err := s.RequestSynastry(ctx, b, a); err != nil || got != "requested" {
			t.Fatalf("%s: the member who declined asks: %q, %v", tc.name, got, err)
		}
		if status, by, at := consentRow(t, fx, a, b); status != "pending" || by != b || at != nil {
			t.Fatalf("%s: %s by %s, answered at %v", tc.name, status, by, at)
		}
		if got := synastryAs(t, s, a, b); got != "pending-in" {
			t.Fatalf("%s: the other member sees %q", tc.name, got)
		}
	}

	// Ending the friendship still removes the consent row in every state.
	addUsers(t, fx, "x", "y")
	befriend(t, fx, "x", "y", "accepted")
	if _, err := s.RequestSynastry(ctx, "x", "y"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.RespondSynastry(ctx, "y", "x", false); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := s.RevokeSynastry(ctx, "x", "y"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := s.RemoveFriend(ctx, "x", "y"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if status, _, _ := consentRow(t, fx, "x", "y"); status != "none" {
		t.Fatalf("consent row is %s after the friendship ended", status)
	}
}

func commentsBetween(t *testing.T, pool *pgxpool.Pool, a, b string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM profile_comments
		 WHERE (profile_id = $1 AND author_id = $2) OR (profile_id = $2 AND author_id = $1)`, a, b).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// leftover stores a comment directly, as one left on a wall before comments
// were removed with the friendship.
func leftover(t *testing.T, pool *pgxpool.Pool, wall, author, body string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO profile_comments (id, profile_id, author_id, body) VALUES ($1, $2, $3, $4)`,
		"wc_"+wall+"_"+author+"_"+body, wall, author, body); err != nil {
		t.Fatal(err)
	}
}

// The pair's wall comments are deleted exactly when a friendship row is: not
// when a declined request is kept, and not when there was nothing to remove.
func TestWallCommentsGoWithTheFriendshipRowAndOnlyThen(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "bob", "cyd", "dee")
	befriend(t, fx, "ann", "bob", "accepted")
	befriend(t, fx, "ann", "cyd", "accepted")
	for _, c := range [][2]string{{"ann", "bob"}, {"bob", "ann"}, {"ann", "cyd"}, {"cyd", "ann"}} {
		if _, err := s.AddWallComment(ctx, c[0], c[1], "hello"); err != nil {
			t.Fatalf("%s on %s's wall: %v", c[1], c[0], err)
		}
	}
	if ok, err := s.RemoveFriend(ctx, "ann", "bob"); err != nil || !ok {
		t.Fatalf("unfriend: %v %v", ok, err)
	}
	if n := commentsBetween(t, fx, "ann", "bob"); n != 0 {
		t.Fatalf("%d comments between ann and bob after the friendship ended", n)
	}
	if n := commentsBetween(t, fx, "ann", "cyd"); n != 2 {
		t.Fatalf("%d comments between ann and cyd, who are still friends; want 2", n)
	}

	// Comments left from an earlier friendship, and a request since declined.
	leftover(t, fx, "ann", "dee", "old1")
	leftover(t, fx, "dee", "ann", "old2")
	// Nothing between them to remove: nothing is deleted.
	if ok, err := s.RemoveFriend(ctx, "ann", "dee"); err != nil || ok {
		t.Fatalf("removing a member who is not a friend: %v %v", ok, err)
	}
	if n := commentsBetween(t, fx, "ann", "dee"); n != 2 {
		t.Fatalf("%d comments left after removing nothing, want 2", n)
	}
	befriend(t, fx, "ann", "dee", "declined")
	// Ann withdraws the request dee declined: the row is kept, and so is
	// everything else.
	if ok, err := s.RemoveFriend(ctx, "ann", "dee"); err != nil || !ok {
		t.Fatalf("withdrawing a declined request: %v %v", ok, err)
	}
	if n := commentsBetween(t, fx, "ann", "dee"); n != 2 || edgeCount(t, fx, "ann", "dee") != 1 {
		t.Fatalf("%d comments and %d rows after the declined member withdrew; want 2 and 1", n, edgeCount(t, fx, "ann", "dee"))
	}
	// Dee, who declined, clears it: the row goes, and the comments with it.
	if ok, err := s.RemoveFriend(ctx, "dee", "ann"); err != nil || !ok {
		t.Fatalf("clearing a declined request: %v %v", ok, err)
	}
	if n := commentsBetween(t, fx, "ann", "dee"); n != 0 || edgeCount(t, fx, "ann", "dee") != 0 {
		t.Fatalf("%d comments and %d rows after the row was removed", n, edgeCount(t, fx, "ann", "dee"))
	}
}

// A wall lists what the owner's friends wrote. A comment by anyone else —
// left from a friendship that ended before comments were removed with it —
// is not listed, on the owner's own read or a friend's.
func TestWallListsOnlyCommentsByCurrentFriends(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "owner", "friend", "former", "asked", "refused", "stranger")
	befriend(t, fx, "friend", "owner", "accepted")
	befriend(t, fx, "owner", "asked", "pending")
	befriend(t, fx, "refused", "owner", "declined")
	if _, err := fx.Exec(ctx, `UPDATE users SET nick = 'FormerNow', avatar = $1, avatar_mime = 'image/png' WHERE id = 'former'`, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	for _, author := range []string{"former", "asked", "refused", "stranger", "friend", "owner"} {
		leftover(t, fx, "owner", author, "by-"+author)
	}
	authors := func() string {
		t.Helper()
		list, err := s.Wall(ctx, "owner", 50)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, c := range list {
			seen[c.AuthorID] = true
			if c.AuthorNick == "FormerNow" && !seen["former"] {
				t.Fatal("a nick without its author")
			}
		}
		out := []string{}
		for _, id := range []string{"asked", "former", "friend", "owner", "refused", "stranger"} {
			if seen[id] {
				out = append(out, id)
			}
		}
		return fmt.Sprint(out)
	}
	if got := authors(); got != "[friend owner]" {
		t.Fatalf("the wall lists comments by %s, want only the friend's and the owner's own", got)
	}
	// Friends again: what they wrote is theirs to show again.
	befriend(t, fx, "owner", "former", "accepted")
	if got := authors(); got != "[former friend owner]" {
		t.Fatalf("after becoming friends again the wall lists %s", got)
	}
}

// Only a friend of the owner can write on a wall, checked where the comment
// is stored; and the limits hold when many comments arrive at once.
func TestAddWallCommentChecksFriendshipAndLimitsItself(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "owner", "friend", "asked", "refused", "stranger")
	befriend(t, fx, "friend", "owner", "accepted")
	befriend(t, fx, "owner", "asked", "pending")
	befriend(t, fx, "refused", "owner", "declined")
	for _, author := range []string{"asked", "refused", "stranger", "owner", "nobody"} {
		if id, err := s.AddWallComment(ctx, "owner", author, "hi"); !errors.Is(err, ErrNotFriends) || id != "" {
			t.Fatalf("%s wrote on the wall: %q, %v", author, id, err)
		}
	}
	if n := commentsBetween(t, fx, "owner", "owner") + commentsBetween(t, fx, "owner", "asked") + commentsBetween(t, fx, "owner", "refused") + commentsBetween(t, fx, "owner", "stranger"); n != 0 {
		t.Fatalf("%d comments stored for members who are not friends", n)
	}

	const callers = maxWallCommentsPerHour + 15
	errs := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = s.AddWallComment(ctx, "owner", "friend", fmt.Sprint("comment ", i))
		}(i)
	}
	close(start)
	wg.Wait()
	stored, refused := 0, 0
	for i, err := range errs {
		switch {
		case err == nil:
			stored++
		case errors.Is(err, ErrWallTooFast):
			refused++
		default:
			t.Fatalf("caller %d: %v", i, err)
		}
	}
	if stored != maxWallCommentsPerHour || refused != callers-maxWallCommentsPerHour || commentsBetween(t, fx, "owner", "friend") != maxWallCommentsPerHour {
		t.Fatalf("%d of %d simultaneous comments stored (%d rows), want %d", stored, callers, commentsBetween(t, fx, "owner", "friend"), maxWallCommentsPerHour)
	}

	// The total limit, reached slowly.
	if _, err := fx.Exec(ctx, `UPDATE profile_comments SET created_at = NOW() - INTERVAL '2 days'`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.Exec(ctx,
		`INSERT INTO profile_comments (id, profile_id, author_id, body, created_at)
		 SELECT 'old_' || g, 'owner', 'friend', 'old', NOW() - INTERVAL '3 days'
		 FROM generate_series(1, $1::int) g`, maxWallCommentsStored-maxWallCommentsPerHour-1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddWallComment(ctx, "owner", "friend", "the last one"); err != nil {
		t.Fatalf("comment %d: %v", maxWallCommentsStored, err)
	}
	if _, err := s.AddWallComment(ctx, "owner", "friend", "one too many"); !errors.Is(err, ErrWallFull) {
		t.Fatalf("comment %d: %v", maxWallCommentsStored+1, err)
	}
	if n := commentsBetween(t, fx, "owner", "friend"); n != maxWallCommentsStored {
		t.Fatalf("%d comments stored, want %d", n, maxWallCommentsStored)
	}
}

// A "no" that is being given at the very moment its sender cancels the
// request must still count: the cancel waits for the answer and then keeps
// the row, instead of deleting a request it took to be waiting — which would
// have thrown the wait away with it.
func TestCancellingWhileTheAnswerIsBeingGivenKeepsTheDecline(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "bob", "cyd")
	befriend(t, fx, "ann", "cyd", "accepted")
	if got, err := s.CreateFriendRequest(ctx, "ann", "bob"); err != nil || got != "created" {
		t.Fatalf("friend request: %q, %v", got, err)
	}
	if got, err := s.RequestSynastry(ctx, "ann", "cyd"); err != nil || got != "requested" {
		t.Fatalf("synastry request: %q, %v", got, err)
	}

	for name, tc := range map[string]struct {
		decline string // the answer, left uncommitted while the cancel starts
		cancel  func() (bool, error)
		status  func() string
	}{
		"friend request": {
			`UPDATE friendships SET status = 'declined', responded_at = NOW() WHERE requester_id = 'ann' AND addressee_id = 'bob'`,
			func() (bool, error) { return s.RemoveFriend(ctx, "ann", "bob") },
			func() string {
				var st string
				if err := fx.QueryRow(ctx, `SELECT status FROM friendships WHERE requester_id = 'ann' AND addressee_id = 'bob'`).Scan(&st); err != nil {
					return err.Error()
				}
				return st
			},
		},
		"chart comparison": {
			`UPDATE synastry_consents SET status = 'declined', responded_at = NOW() WHERE requested_by = 'ann'`,
			func() (bool, error) { return s.RevokeSynastry(ctx, "ann", "cyd") },
			func() string { st, _, _ := consentRow(t, fx, "ann", "cyd"); return st },
		},
	} {
		answer, err := fx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := answer.Exec(ctx, tc.decline); err != nil {
			t.Fatal(err)
		}
		type outcome struct {
			ok  bool
			err error
		}
		done := make(chan outcome, 1)
		go func() {
			ok, err := tc.cancel()
			done <- outcome{ok, err}
		}()
		select {
		case got := <-done:
			_ = answer.Rollback(ctx)
			t.Fatalf("%s: the cancel did not wait for the answer in progress: %+v", name, got)
		case <-time.After(300 * time.Millisecond):
		}
		if err := answer.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if got := <-done; got.err != nil || !got.ok {
			t.Fatalf("%s: cancel: %+v", name, got)
		}
		if st := tc.status(); st != "withdrawn" {
			t.Fatalf("%s: after a decline and a cancel at the same moment the row is %q, want it kept as withdrawn", name, st)
		}
	}
}

// WithdrawFriendRequest acts on one request the caller sent, while it is
// still a request, and leaves everything else between the two alone.
func TestWithdrawFriendRequestTouchesOnlyTheRequestNamed(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "pend", "decl", "acc", "gone", "in", "third")
	befriend(t, fx, "ann", "pend", "pending")
	befriend(t, fx, "ann", "decl", "declined")
	befriend(t, fx, "ann", "acc", "accepted")
	befriend(t, fx, "ann", "gone", "withdrawn")
	befriend(t, fx, "in", "ann", "pending")
	befriend(t, fx, "ann", "third", "pending")
	if _, err := fx.Exec(ctx, `UPDATE friendships SET responded_at = NOW() - INTERVAL '1 day' WHERE status IN ('declined', 'withdrawn', 'accepted')`); err != nil {
		t.Fatal(err)
	}
	// Things that exist between ann and the others, none of them the request.
	if _, err := s.RequestSynastry(ctx, "ann", "acc"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddWallComment(ctx, "acc", "ann", "hello"); err != nil {
		t.Fatal(err)
	}
	leftover(t, fx, "ann", "pend", "from-an-earlier-friendship")
	rows := func() string {
		t.Helper()
		var all string
		if err := fx.QueryRow(ctx, `SELECT string_agg(id || ':' || status || ':' || COALESCE(responded_at::text, '-'), ',' ORDER BY id) FROM friendships`).Scan(&all); err != nil {
			t.Fatal(err)
		}
		return all
	}
	withdraw := func(other, id string) bool {
		t.Helper()
		ok, err := s.WithdrawFriendRequest(ctx, "ann", other, id)
		if err != nil {
			t.Fatalf("withdraw %s to %s: %v", id, other, err)
		}
		return ok
	}
	before := rows()

	for name, try := range map[string][2]string{
		"an accepted request":                 {"acc", "fr_ann_acc"},
		"a request already withdrawn":         {"gone", "fr_ann_gone"},
		"a request received":                  {"in", "fr_in_ann"},
		"an id of another pair":               {"pend", "fr_ann_third"},
		"the right id under the wrong member": {"third", "fr_ann_pend"},
		"no id":                               {"pend", ""},
		"an unknown id":                       {"pend", "fr_ann_pend "},
		"an unknown member":                   {"nobody", "fr_ann_pend"},
	} {
		if withdraw(try[0], try[1]) || rows() != before {
			t.Fatalf("%s was acted on:\nbefore %s\nafter  %s", name, before, rows())
		}
	}
	if status, _, _ := consentRow(t, fx, "ann", "acc"); status != "pending" || commentsBetween(t, fx, "ann", "acc") != 1 {
		t.Fatalf("the friendship with acc lost something: consent %s, %d comments", status, commentsBetween(t, fx, "ann", "acc"))
	}

	// A waiting request is deleted; a declined one is kept as withdrawn with
	// the time of its decline. Each only once.
	var declinedAt *time.Time
	if err := fx.QueryRow(ctx, `SELECT responded_at FROM friendships WHERE id = 'fr_ann_decl'`).Scan(&declinedAt); err != nil || declinedAt == nil {
		t.Fatalf("no time of decline: %v %v", declinedAt, err)
	}
	if !withdraw("pend", "fr_ann_pend") || !withdraw("decl", "fr_ann_decl") {
		t.Fatal("a request that could be cancelled was not")
	}
	if withdraw("pend", "fr_ann_pend") || withdraw("decl", "fr_ann_decl") {
		t.Fatal("a request was cancelled twice")
	}
	if n := edgeCount(t, fx, "ann", "pend"); n != 0 {
		t.Fatalf("%d rows left of the waiting request", n)
	}
	var status string
	var at *time.Time
	if err := fx.QueryRow(ctx, `SELECT status, responded_at FROM friendships WHERE id = 'fr_ann_decl'`).Scan(&status, &at); err != nil || status != "withdrawn" || at == nil || !at.Equal(*declinedAt) {
		t.Fatalf("the declined request is %q, answered at %v (declined at %v), %v", status, at, declinedAt, err)
	}
	// Only the request row went: what an earlier friendship left on the wall
	// is not this call's to remove.
	if n := commentsBetween(t, fx, "ann", "pend"); n != 1 {
		t.Fatalf("%d comments between ann and pend, want the 1 that was there", n)
	}
	for _, id := range []string{"fr_ann_acc", "fr_ann_gone", "fr_in_ann", "fr_ann_third"} {
		if !strings.Contains(rows(), id+":") || !strings.Contains(before, id+":") {
			t.Fatalf("%s is missing: %s", id, rows())
		}
	}
}

// The request is accepted, or declined, at the very moment its sender cancels
// it. The cancel waits for the answer and is then decided on what the request
// has become: an accepted one is left alone, a declined one is kept.
func TestWithdrawFriendRequestWaitsForAnAnswerInProgress(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "ann", "yes", "no")
	befriend(t, fx, "ann", "yes", "pending")
	befriend(t, fx, "ann", "no", "pending")

	for _, tc := range []struct {
		other, answer string
		wantOK        bool
		wantStatus    string
	}{
		{"yes", "accepted", false, "accepted"},
		{"no", "declined", true, "withdrawn"},
	} {
		id := "fr_ann_" + tc.other
		answer, err := fx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := answer.Exec(ctx, `UPDATE friendships SET status = $2, responded_at = NOW() WHERE id = $1`, id, tc.answer); err != nil {
			t.Fatal(err)
		}
		type outcome struct {
			ok  bool
			err error
		}
		done := make(chan outcome, 1)
		go func() {
			ok, err := s.WithdrawFriendRequest(ctx, "ann", tc.other, id)
			done <- outcome{ok, err}
		}()
		select {
		case got := <-done:
			_ = answer.Rollback(ctx)
			t.Fatalf("%s: the cancel did not wait for the answer in progress: %+v", tc.answer, got)
		case <-time.After(300 * time.Millisecond):
		}
		if err := answer.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if got := <-done; got.err != nil || got.ok != tc.wantOK {
			t.Fatalf("%s while cancelling: %+v, want ok=%v", tc.answer, got, tc.wantOK)
		}
		var status string
		if err := fx.QueryRow(ctx, `SELECT status FROM friendships WHERE id = $1`, id).Scan(&status); err != nil || status != tc.wantStatus {
			t.Fatalf("%s while cancelling: the row is %q (%v), want %q", tc.answer, status, err, tc.wantStatus)
		}
	}
}

// SentFriendRequestsFull answers from what the member has listed as sent:
// waiting requests and declined ones (shown to them as waiting). Nothing else
// about the member, and nothing about anyone else, changes the answer.
func TestSentFriendRequestsFullCountsOnlyTheSendersList(t *testing.T) {
	s, fx := newTestStore(t)
	ctx := context.Background()
	addUsers(t, fx, "sender", "idle")
	bulkUsers(t, fx, "to", MaxSentFriendRequests+10, nil)
	bulkUsers(t, fx, "from", MaxSentFriendRequests+10, nil)
	full := func(member string) bool {
		t.Helper()
		got, err := s.SentFriendRequestsFull(ctx, member)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	set := func(n int, status string) {
		t.Helper()
		if _, err := fx.Exec(ctx, `UPDATE friendships SET status = $2 WHERE id = $1`, fmt.Sprintf("fr_to%04d", n), status); err != nil {
			t.Fatal(err)
		}
	}

	// Requests received, however many, are not requests sent.
	bulkEdges(t, fx, "from", MaxSentFriendRequests+10, "sender", "pending", false)
	if full("sender") || full("idle") || full("nobody") {
		t.Fatal("a member who sent nothing is at the limit")
	}
	bulkEdges(t, fx, "to", MaxSentFriendRequests-1, "sender", "pending", true)
	if full("sender") {
		t.Fatalf("at the limit with %d requests sent", MaxSentFriendRequests-1)
	}
	if _, err := fx.Exec(ctx,
		`INSERT INTO friendships (id, requester_id, addressee_id, status) VALUES ($1, 'sender', $2, 'pending')`,
		fmt.Sprintf("fr_to%04d", MaxSentFriendRequests), fmt.Sprintf("to%04d", MaxSentFriendRequests)); err != nil {
		t.Fatal(err)
	}
	if !full("sender") {
		t.Fatalf("not at the limit with %d requests waiting", MaxSentFriendRequests)
	}
	// A declined request is still on the sender's list and still counts; one
	// they withdrew, and one that was accepted, are off it.
	set(1, "declined")
	if !full("sender") {
		t.Fatal("a declined request stopped counting: the limit would give the decline away")
	}
	set(1, "withdrawn")
	if full("sender") {
		t.Fatal("a withdrawn request still counts")
	}
	set(1, "pending")
	set(2, "accepted")
	if full("sender") {
		t.Fatal("an accepted request still counts")
	}
	if full("idle") {
		t.Fatal("another member's requests count against this one")
	}
}
