// Social surface of the store: friendships, dashboard prefs, and the birth
// data needed to compute charts. Same house rules as store.go — parameterized
// queries only, and birth data is read here to be COMPUTED ON, never to be
// returned by an API handler.
package socialstore

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrSelfFriend = errors.New("store: cannot befriend yourself")

// ── users ────────────────────────────────────────────────────────────────────

type PublicUser struct {
	ID    string `json:"userId"`
	Email string `json:"email"`
}

func (s *Store) UserEmail(ctx context.Context, userID string) (string, error) {
	var email string
	err := s.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	return email, err
}

// BirthMoment reconstructs the user's birth instant (UTC) and place exactly
// the way the TS server does: local wall time minus the stored tz offset.
// Never serialize this struct into a response.
type BirthMoment struct {
	UTC       time.Time
	Latitude  float64
	Longitude float64
}

func (s *Store) BirthMoment(ctx context.Context, userID string) (*BirthMoment, error) {
	var dateStr, timeStr *string
	var tzOff, lat, lon *float64
	err := s.pool.QueryRow(ctx,
		`SELECT birth_date::text, birth_time::text, birth_timezone_offset, birth_latitude, birth_longitude
		 FROM users WHERE id = $1`, userID,
	).Scan(&dateStr, &timeStr, &tzOff, &lat, &lon)
	if err != nil {
		return nil, err
	}
	if dateStr == nil || *dateStr == "" {
		return nil, errors.New("no birth data")
	}
	ts := ""
	if timeStr != nil {
		ts = *timeStr
	}
	utc, err := birthUTC(*dateStr, ts, tzOff)
	if err != nil {
		return nil, err
	}
	bm := &BirthMoment{UTC: utc}
	if lat != nil {
		bm.Latitude = *lat
	}
	if lon != nil {
		bm.Longitude = *lon
	}
	return bm, nil
}

// birthUTC turns the stored local birth date, wall-clock time and timezone
// offset into the UTC instant.
//
// The offset is hours east of UTC; values beyond ±14 are legacy rows that
// stored minutes with the opposite sign. It is applied in whole MINUTES,
// rounded the way the Rust API rounds it (half away from zero): converting
// the hours to an integer first, as this code used to, dropped the half or
// three-quarter hour of every such timezone and moved the birth instant by
// 30 or 45 minutes.
func birthUTC(date, clock string, tzOff *float64) (time.Time, error) {
	ds := date
	if len(ds) > 10 {
		ds = ds[:10] // birth_date may be a DATE or a timestamp::text
	}
	var y, m, d int
	if _, err := fmt.Sscanf(ds, "%d-%d-%d", &y, &m, &d); err != nil {
		return time.Time{}, fmt.Errorf("bad birth_date %q", ds)
	}
	hh, mm := 12, 0 // an unknown birth time is taken as local noon
	if len(clock) >= 5 {
		_, _ = fmt.Sscanf(clock[:5], "%d:%d", &hh, &mm)
	}
	off := 0.0
	if tzOff != nil {
		off = *tzOff
		// legacy rows stored minutes; the TS server normalizes the same way
		if off > 14 || off < -14 {
			off = -(off / 60)
		}
	}
	offset := time.Duration(math.Round(off*60)) * time.Minute
	local := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC).
		Add(time.Duration(hh) * time.Hour).
		Add(time.Duration(mm) * time.Minute)
	return local.Add(-offset), nil
}

// ── dashboard prefs (JSONB, opaque to the store) ────────────────────────────

func (s *Store) DashboardPrefs(ctx context.Context, userID string) (json.RawMessage, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT dashboard_prefs FROM users WHERE id = $1`, userID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return json.RawMessage(`null`), nil
	}
	return json.RawMessage(raw), nil
}

func (s *Store) SetDashboardPrefs(ctx context.Context, userID string, prefs json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET dashboard_prefs = $2 WHERE id = $1`, userID, prefs)
	return err
}

// ── friendships ──────────────────────────────────────────────────────────────

type FriendEntry struct {
	UserID string `json:"userId"`
	Nick   string `json:"nick"`
	Since  string `json:"since,omitempty"`
	// Synastry consent state relative to the viewer:
	// none | pending-out | pending-in | accepted | declined
	Synastry string `json:"synastry"`
	Avatar   string `json:"avatar,omitempty"`
}

// FriendRequestEntry is a request that is waiting, in either direction. It has
// no picture on purpose: anyone can send a request, so a picture here is
// reply size that strangers control.
type FriendRequestEntry struct {
	RequestID string `json:"requestId"`
	UserID    string `json:"userId"`
	Nick      string `json:"nick"`
}

type Friendships struct {
	Accepted []FriendEntry        `json:"accepted"`
	Incoming []FriendRequestEntry `json:"incoming"`
	Outgoing []FriendRequestEntry `json:"outgoing"`
}

// Bounds on one friends-list reply. A reply that grew without limit could
// stop being delivered, and a member who cannot open the list cannot even
// decline the requests that filled it. Requests cost nothing to send, so their
// part of the list has to be small whatever anyone does: no pictures, and a
// fixed number of rows.
const (
	// maxFriendsListed is how many accepted friends one reply lists.
	maxFriendsListed = 500
	// maxPendingListed is how many requests one reply lists in each
	// direction, newest first.
	maxPendingListed = 100
	// friendListAvatarBudget is how many bytes of pictures one reply carries
	// (about a third more once base64-encoded). Friends past it are still
	// listed, without a picture.
	friendListAvatarBudget = 2 << 20
	// MaxSentFriendRequests is how many requests one member may have waiting
	// for an answer at a time. Each one is a row in someone else's list.
	// Exported for the handler, which names the number when it refuses.
	MaxSentFriendRequests = 50
)

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(b))
}

// friendPairLockKey is the advisory-lock key for an unordered pair of users:
// the same value for (a,b) and (b,a). It is FNV-1a over a namespaced,
// NUL-separated encoding of the ordered pair, so every instance and every
// release computes the same key — a key that differed between two processes
// would exclude nothing. (Postgres text cannot contain NUL, so the separator
// cannot be forged by an id.)
func friendPairLockKey(a, b string) int64 {
	lo, hi := pairLoHi(a, b)
	h := fnv.New64a()
	_, _ = h.Write([]byte("friendships\x00"))
	_, _ = h.Write([]byte(lo))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(hi))
	return int64(h.Sum64())
}

// sentRequestsLockKey is the advisory-lock key for "the requests this member
// has sent", in a namespace of its own so that it never equals a pair's key
// by construction.
func sentRequestsLockKey(requesterID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("friend-requests-sent\x00"))
	_, _ = h.Write([]byte(requesterID))
	return int64(h.Sum64())
}

// rowQuerier is the one method the count below needs. The pool and a
// transaction both have it.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// sentRequestsAtLimit reports whether a member already has
// MaxSentFriendRequests requests listed as sent.
//
// A declined request counts as long as it is on their list. They are shown it
// as waiting, and if it did not count, one more request going through than the
// list has room for would tell them that something on it had been declined.
// The answer depends on the member's own rows and on nothing else.
func sentRequestsAtLimit(ctx context.Context, q rowQuerier, requesterID string) (bool, error) {
	var listed int
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM (
		     SELECT 1 FROM friendships
		     WHERE requester_id = $1 AND status IN ('pending', 'declined')
		     LIMIT $2) sent`,
		requesterID, MaxSentFriendRequests).Scan(&listed)
	if err != nil {
		return false, err
	}
	return listed >= MaxSentFriendRequests, nil
}

// sentRequestsFull is the authoritative check, made inside the transaction
// that is about to add a request to the member's list.
//
// The count and whatever the caller then writes must not interleave with
// another request from the same member, or two could both see room for one.
// The transaction-scoped lock taken here makes them queue. It is always taken
// after the pair lock and nothing else is acquired while holding it, so it
// cannot be part of a deadlock.
func sentRequestsFull(ctx context.Context, tx pgx.Tx, requesterID string) (bool, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, sentRequestsLockKey(requesterID)); err != nil {
		return false, err
	}
	return sentRequestsAtLimit(ctx, tx, requesterID)
}

// SentFriendRequestsFull is the same question asked up front, so that a member
// at the limit can be told so instead of having a request quietly dropped.
// It reads the member's own count and takes no lock: CreateFriendRequest
// checks again, under its locks, and that check is the one that holds when
// several requests arrive at once.
func (s *Store) SentFriendRequestsFull(ctx context.Context, requesterID string) (bool, error) {
	return sentRequestsAtLimit(ctx, s.pool, requesterID)
}

// friendRequestCooldown is how long someone whose friend request was declined
// waits before the same request can be made again.
const friendRequestCooldown = 7 * 24 * time.Hour

// A friendship row is in one of four states:
//
//   - pending:   a request waiting for the addressee's answer.
//   - accepted:  the two are friends.
//   - declined:  the addressee said no. The row is kept, because it is what
//     makes the requester wait before asking again.
//   - withdrawn: declined, and the requester has since withdrawn the request.
//     Still a "no" with the same wait; the row only stops being
//     listed to the requester.
//
// The member who was declined is never shown the difference between "waiting"
// and "declined". A declined request stays in their list of requests sent,
// leaves it when they withdraw it, and comes back when they send it again,
// exactly as a waiting one would. The member who declined sees nothing of it.

// CreateFriendRequest inserts one pending edge unless a row already exists in
// either direction. Returns "created" | "exists".
//
// A declined edge is the one kind that can be asked about again: at once by
// the member who declined, and after friendRequestCooldown by the member who
// was declined. The pair's row is reused, so there is still one row per pair.
//
// One member may have MaxSentFriendRequests requests waiting at a time. Past
// that nothing is stored and the answer is "exists", the same as for a request
// that is already there. A caller that wants to tell the member asks
// SentFriendRequestsFull first; the check here is what holds in a race.
//
// The table's unique constraint covers one direction only, so "does an edge
// exist?" followed by an insert is a race: two members asking each other at
// once both passed the check and left two rows, and a double click hit the
// constraint as an error. The check and the insert therefore run in one
// transaction holding a transaction-scoped advisory lock on the unordered
// pair. A second caller waits, then sees the first caller's row and reports
// "exists". No schema change is involved.
func (s *Store) CreateFriendRequest(ctx context.Context, requesterID, addresseeID string) (string, error) {
	if requesterID == addresseeID {
		return "", ErrSelfFriend
	}
	// READ COMMITTED on purpose: the existence check must take its snapshot
	// after the lock is granted, or it could not see the row the previous
	// holder just committed.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`,
		friendPairLockKey(requesterID, addresseeID)); err != nil {
		return "", err
	}
	// Every edge of the pair, in either direction, locked for this decision.
	rows, err := tx.Query(ctx,
		`SELECT id, status, requester_id,
		        (responded_at IS NULL OR responded_at <= NOW() - make_interval(secs => $3)) AS cooled
		 FROM friendships
		 WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)
		 FOR UPDATE`,
		requesterID, addresseeID, friendRequestCooldown.Seconds())
	if err != nil {
		return "", err
	}
	type edge struct {
		id, status, requester string
		cooled                bool
	}
	var edges []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.id, &e.status, &e.requester, &e.cooled); err != nil {
			rows.Close()
			return "", err
		}
		edges = append(edges, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(edges) > 0 {
		// A pending or accepted edge stands. So does anything unexpected, such
		// as two edges for one pair: it is left exactly as it is.
		if len(edges) != 1 || (edges[0].status != "declined" && edges[0].status != "withdrawn") {
			return "exists", nil
		}
		declined := edges[0]
		sameAsker := declined.requester == requesterID
		withdrawn := declined.status == "withdrawn"
		// The one who declined may ask in their own turn whenever they like:
		// they changed their mind. The one who was declined may ask again once
		// the cooldown has passed, so a "no" is not something to be worn down
		// by asking every day. Until then the answer is the same as for a
		// request that is still waiting, which keeps the decline private.
		if sameAsker && !declined.cooled {
			if !withdrawn {
				return "exists", nil
			}
			// They withdrew it and are sending it again. Nothing reaches the
			// other member and the wait is not touched, but to the sender it
			// has to look like the new request it would be had it only been
			// waiting: listed again, under a new id.
			full, err := sentRequestsFull(ctx, tx, requesterID)
			if err != nil {
				return "", err
			}
			if full {
				return "exists", nil
			}
			if _, err := tx.Exec(ctx,
				`UPDATE friendships SET id = $2, status = 'declined' WHERE id = $1 AND status = 'withdrawn'`,
				declined.id, newID("fr")); err != nil {
				return "", err
			}
			if err := tx.Commit(ctx); err != nil {
				return "", err
			}
			return "exists", nil
		}
		// The pair keeps its one row; it becomes a fresh request from this
		// requester. The status guard makes a concurrent change a no-op.
		//
		// It keeps its id only when the same member asks again about a request
		// that never left their list: to them nothing may change. In the other
		// cases it is a new request for whoever sends it, and gets a new id
		// like one.
		id := declined.id
		if !sameAsker || withdrawn {
			// New on this member's list of requests sent, so it needs room
			// there.
			full, err := sentRequestsFull(ctx, tx, requesterID)
			if err != nil {
				return "", err
			}
			if full {
				return "exists", nil
			}
			id = newID("fr")
		}
		tag, err := tx.Exec(ctx,
			`UPDATE friendships
			 SET id = $4, requester_id = $2, addressee_id = $3, status = 'pending', created_at = NOW(), responded_at = NULL
			 WHERE id = $1 AND status IN ('declined', 'withdrawn')`,
			declined.id, requesterID, addresseeID, id)
		if err != nil {
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		if tag.RowsAffected() == 0 {
			return "exists", nil
		}
		return "created", nil
	}
	full, err := sentRequestsFull(ctx, tx, requesterID)
	if err != nil {
		return "", err
	}
	if full {
		return "exists", nil
	}
	// ON CONFLICT is the backstop for a writer that does not take the lock
	// (an older instance during a deploy): a duplicate is an outcome, never
	// an error.
	tag, err := tx.Exec(ctx,
		`INSERT INTO friendships (id, requester_id, addressee_id, status) VALUES ($1, $2, $3, 'pending')
		 ON CONFLICT DO NOTHING`,
		newID("fr"), requesterID, addresseeID)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "exists", nil
	}
	return "created", nil
}

func (s *Store) RespondFriendRequest(ctx context.Context, requestID, addresseeID string, accept bool) (bool, error) {
	status := "declined"
	if accept {
		status = "accepted"
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE friendships SET status = $3, responded_at = NOW()
		 WHERE id = $1 AND addressee_id = $2 AND status = 'pending'`,
		requestID, addresseeID, status)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) RemoveFriend(ctx context.Context, userID, otherID string) (bool, error) {
	// One transaction: ending the friendship MUST also end any synastry
	// consent between the pair — a stale accepted row surviving an unfriend
	// would silently unlock synastry again on re-friending.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The same lock a new request takes, so that a request and a removal of
	// the same pair are decided one after the other.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, friendPairLockKey(userID, otherID)); err != nil {
		return false, err
	}
	// What the caller sent the other, read under a row lock: an answer that
	// is being given at this moment is waited for, so the decision below is
	// made on the request as it really is, never on its state a moment ago.
	var sentID, sentStatus string
	err = tx.QueryRow(ctx,
		`SELECT id, status FROM friendships
		 WHERE requester_id = $1 AND addressee_id = $2
		 FOR UPDATE`, userID, otherID).Scan(&sentID, &sentStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	switch sentStatus {
	case "declined":
		// A request this member sent and the other declined stays where it
		// is. Deleting it would let them ask again at once, which the wait
		// after a decline exists to prevent. To them it looks like a request
		// that is waiting, so withdrawing it does what withdrawing one of
		// those does: it reports success and the request leaves their list.
		// Only the state changes; the row and the time of the decline are
		// kept.
		if _, err := tx.Exec(ctx, `UPDATE friendships SET status = 'withdrawn' WHERE id = $1`, sentID); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	case "withdrawn":
		// One they already withdrew is, as far as they can tell, not there.
		return false, nil
	}
	tag, err := tx.Exec(ctx,
		`DELETE FROM friendships
		 WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)`,
		userID, otherID)
	if err != nil {
		return false, err
	}
	lo, hi := pairLoHi(userID, otherID)
	if _, err := tx.Exec(ctx, `DELETE FROM synastry_consents WHERE user_lo = $1 AND user_hi = $2`, lo, hi); err != nil {
		return false, err
	}
	// What each of the two wrote on the other's wall goes with the edge. A
	// wall shows a comment with its author's nick and picture as they are
	// NOW, so a comment left behind kept showing each of them the other's
	// current name and face after the friendship was over.
	if tag.RowsAffected() > 0 {
		if _, err := tx.Exec(ctx,
			`DELETE FROM profile_comments
			 WHERE (profile_id = $1 AND author_id = $2) OR (profile_id = $2 AND author_id = $1)`,
			userID, otherID); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// WithdrawFriendRequest cancels one request the caller sent, named by its id,
// and nothing else. It is what a "Cancel" control beside a sent request must
// call: the control was drawn from a list read some time ago, and RemoveFriend
// would by then end a friendship if the request had been accepted meanwhile.
//
// It acts only on a row with that id that the caller sent to otherID:
//
//   - pending:  the row is deleted. The request is gone.
//   - declined: the row is kept and marked withdrawn, exactly as RemoveFriend
//     does, so the wait after the decline stays in force. The caller
//     cannot tell this case from the first.
//
// In every other case — accepted, already withdrawn, no such row, an id that
// belongs to another pair or to a request the caller received — nothing is
// changed and the result is false. Only that one row is ever touched: the
// pair's consent row and wall comments are left to RemoveFriend.
//
// Same discipline as RemoveFriend: the pair's lock, then the row read under a
// row lock, so an answer that is being given at this moment is waited for and
// the decision is made on the request as it really is.
func (s *Store) WithdrawFriendRequest(ctx context.Context, userID, otherID, requestID string) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, friendPairLockKey(userID, otherID)); err != nil {
		return false, err
	}
	var status string
	err = tx.QueryRow(ctx,
		`SELECT status FROM friendships
		 WHERE id = $3 AND requester_id = $1 AND addressee_id = $2
		 FOR UPDATE`, userID, otherID, requestID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// The row is locked and its state known; the state is repeated in each
	// statement all the same, so neither can ever act on anything else, and
	// the result says what was actually changed.
	var change string
	switch status {
	case "pending":
		change = `DELETE FROM friendships WHERE id = $1 AND status = 'pending'`
	case "declined":
		change = `UPDATE friendships SET status = 'withdrawn' WHERE id = $1 AND status = 'declined'`
	default:
		return false, nil
	}
	tag, err := tx.Exec(ctx, change, requestID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) AreFriends(ctx context.Context, a, b string) (bool, error) {
	var one int
	err := s.pool.QueryRow(ctx,
		`SELECT 1 FROM friendships
		 WHERE ((requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1))
		   AND status = 'accepted'`, a, b).Scan(&one)
	if err != nil {
		return false, nil //nolint:nilerr // no row = not friends
	}
	return true, nil
}

// ListFriendships loads relationships and their consent state in one query.
//
// Every part of the reply is bounded (see maxFriendsListed): accepted friends
// in the order the friendships began, then the newest requests received and
// the newest sent. Requests carry a nick and no picture, in either direction.
//
// Requests sent include the ones that were declined: the member who was
// declined is shown a request that is waiting. They are ordered by id, which
// begins with the moment the request was made (compared byte by byte, so the
// database's collation has no say). created_at would not do: it
// moves when a declined request is reopened after the wait, and the entry
// jumping to the top of the sender's list would tell them it had been
// declined.
//
// A friend's picture is sent while friendListAvatarBudget lasts; a stored
// picture over MaxAvatarBytes is left out of every row (see there) and uses
// none of the budget. The sizes are added up in SQL, so a picture that is not
// sent is never read out of the table.
func (s *Store) ListFriendships(ctx context.Context, userID string) (*Friendships, error) {
	out := &Friendships{Accepted: []FriendEntry{}, Incoming: []FriendRequestEntry{}, Outgoing: []FriendRequestEntry{}}
	const accepted, incoming, outgoing = 0, 1, 2
	rows, err := s.pool.Query(ctx, `
		WITH listed AS (
			(SELECT 0 AS section, ROW_NUMBER() OVER (ORDER BY p.created_at, p.id) AS place,
			        p.id, p.responded_at, p.other_id
			 FROM (SELECT f.id, f.created_at, f.responded_at,
			              CASE WHEN f.requester_id = $1 THEN f.addressee_id ELSE f.requester_id END AS other_id
			       FROM friendships f
			       WHERE (f.requester_id = $1 OR f.addressee_id = $1) AND f.status = 'accepted'
			       ORDER BY f.created_at, f.id LIMIT $3) p)
			UNION ALL
			(SELECT 1, ROW_NUMBER() OVER (ORDER BY p.created_at DESC, p.id DESC),
			        p.id, p.responded_at, p.other_id
			 FROM (SELECT f.id, f.created_at, f.responded_at, f.requester_id AS other_id
			       FROM friendships f
			       WHERE f.addressee_id = $1 AND f.status = 'pending'
			       ORDER BY f.created_at DESC, f.id DESC LIMIT $4) p)
			UNION ALL
			(SELECT 2, ROW_NUMBER() OVER (ORDER BY p.id COLLATE "C" DESC),
			        p.id, p.responded_at, p.other_id
			 FROM (SELECT f.id, f.responded_at, f.addressee_id AS other_id
			       FROM friendships f
			       WHERE f.requester_id = $1 AND f.status IN ('pending', 'declined')
			       ORDER BY f.id COLLATE "C" DESC LIMIT $4) p)
		), sized AS (
			SELECT l.section, l.place, l.id, l.responded_at, l.other_id, u.nick, u.avatar, u.avatar_mime,
			       CASE WHEN l.section = 0 AND octet_length(u.avatar) <= $2
			            THEN octet_length(u.avatar) ELSE 0 END AS picture_bytes
			FROM listed l JOIN users u ON u.id = l.other_id
		), budgeted AS (
			SELECT z.*, SUM(z.picture_bytes) OVER (ORDER BY z.section, z.place
			                                       ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS spent
			FROM sized z
		)
		SELECT b.section, b.id, b.other_id, b.nick, COALESCE(b.responded_at::text, ''),
		       CASE WHEN b.picture_bytes > 0 AND b.spent <= $5 THEN b.avatar END,
		       CASE WHEN b.picture_bytes > 0 AND b.spent <= $5 THEN b.avatar_mime END,
		       COALESCE(c.status, 'none'), COALESCE(c.requested_by, '')
		FROM budgeted b
		LEFT JOIN synastry_consents c ON b.section = 0 AND
		    ((c.user_lo = $1 AND c.user_hi = b.other_id) OR (c.user_hi = $1 AND c.user_lo = b.other_id))
		ORDER BY b.section, b.place`,
		userID, MaxAvatarBytes, maxFriendsListed, maxPendingListed, friendListAvatarBudget)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var section int
		var requestID, otherID, since, consent, requestedBy string
		var nick, mime *string
		var avatar []byte
		if err := rows.Scan(&section, &requestID, &otherID, &nick, &since,
			&avatar, &mime, &consent, &requestedBy); err != nil {
			return nil, err
		}
		name := displayName(nick, otherID)
		switch section {
		case accepted:
			// A decline stays hidden from the requester: see SeenBy.
			shown := (&SynastryConsent{Status: consent, RequestedBy: requestedBy}).SeenBy(userID)
			out.Accepted = append(out.Accepted, FriendEntry{UserID: otherID, Nick: name, Since: since, Synastry: shown, Avatar: avatarDataURL(avatar, mime)})
		case incoming:
			out.Incoming = append(out.Incoming, FriendRequestEntry{RequestID: requestID, UserID: otherID, Nick: name})
		case outgoing:
			out.Outgoing = append(out.Outgoing, FriendRequestEntry{RequestID: requestID, UserID: otherID, Nick: name})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ── synastry consent ─────────────────────────────────────────────────────────
// One row per unordered pair. Synastry runs only when status = 'accepted'.
// A request from each side counts as mutual acceptance.
//
// A declined request follows the rules of a declined friend request. The
// member who declined may ask in their own turn at any time; the member who
// was declined only friendRequestCooldown after the decline. And the member
// who was declined is not shown the decline: to them the request goes on
// looking like one that is waiting, until they cancel it. A request they
// cancelled after it was declined is kept in the state 'withdrawn': still a
// "no" with its wait, shown to them as no request at all.

func pairLoHi(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

type SynastryConsent struct {
	Status      string // none | pending | accepted | declined | withdrawn
	RequestedBy string // empty when Status == none
}

// SeenBy is the consent as one member of the pair is shown it:
// none | pending-out | pending-in | accepted | declined.
//
// "declined" is only ever shown to the member who declined. The member whose
// request it was sees it as still waiting ("pending-out"), or as nothing once
// they have cancelled it.
func (c *SynastryConsent) SeenBy(viewer string) string {
	mine := c.RequestedBy == viewer
	switch c.Status {
	case "pending":
		if mine {
			return "pending-out"
		}
		return "pending-in"
	case "accepted":
		return "accepted"
	case "declined":
		if mine {
			return "pending-out"
		}
		return "declined"
	case "withdrawn":
		if mine {
			return "none"
		}
		return "declined"
	}
	return "none"
}

func (s *Store) SynastryConsentState(ctx context.Context, a, b string) (*SynastryConsent, error) {
	lo, hi := pairLoHi(a, b)
	var status, reqBy string
	err := s.pool.QueryRow(ctx,
		`SELECT status, requested_by FROM synastry_consents WHERE user_lo = $1 AND user_hi = $2`,
		lo, hi).Scan(&status, &reqBy)
	if err != nil {
		return &SynastryConsent{Status: "none"}, nil //nolint:nilerr // no row = no consent yet
	}
	return &SynastryConsent{Status: status, RequestedBy: reqBy}, nil
}

// RequestSynastry moves the pair's consent forward from the caller's side.
// Returns "requested" | "accepted" | "already-requested" | "already-accepted".
//
// The answer is the one the caller would get if a request of theirs that was
// declined were still waiting: "already-requested" while it is on their list,
// "requested" once they have cancelled it, whether or not anything was sent.
func (s *Store) RequestSynastry(ctx context.Context, me, other string) (string, error) {
	lo, hi := pairLoHi(me, other)
	cur := SynastryConsent{Status: "none"}
	var cooled bool
	err := s.pool.QueryRow(ctx,
		`SELECT status, requested_by,
		        (responded_at IS NULL OR responded_at <= NOW() - make_interval(secs => $3))
		 FROM synastry_consents WHERE user_lo = $1 AND user_hi = $2`,
		lo, hi, friendRequestCooldown.Seconds()).Scan(&cur.Status, &cur.RequestedBy, &cooled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	switch cur.Status {
	case "none":
		_, err := s.pool.Exec(ctx,
			`INSERT INTO synastry_consents (id, user_lo, user_hi, requested_by, status)
			 VALUES ($1, $2, $3, $4, 'pending')
			 ON CONFLICT (user_lo, user_hi) DO NOTHING`,
			newID("sy"), lo, hi, me)
		if err != nil {
			return "", err
		}
		return "requested", nil
	case "pending":
		if cur.RequestedBy == me {
			return "already-requested", nil
		}
		// They asked, I ask back — that is mutual acceptance. The requested_by
		// guard makes this safe against the read racing a revoke + re-request:
		// it can never flip the caller's OWN request to accepted.
		tag, err := s.pool.Exec(ctx,
			`UPDATE synastry_consents SET status = 'accepted', responded_at = NOW()
			 WHERE user_lo = $1 AND user_hi = $2 AND status = 'pending' AND requested_by <> $3`,
			lo, hi, me)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() == 0 {
			return "already-requested", nil // state moved under us; non-escalating answer
		}
		return "accepted", nil
	case "accepted":
		return "already-accepted", nil
	}

	// Declined, or declined and then cancelled by the member who had asked.
	wasDeclined := cur.RequestedBy == me // the caller is the one who was told no
	cancelled := cur.Status == "withdrawn"
	// What the caller is told. A request that never left their list was
	// "already requested" as far as they know; anything else is a request
	// they are making now.
	answer := "requested"
	if wasDeclined && !cancelled {
		answer = "already-requested"
	}
	if wasDeclined && !cooled {
		// Inside the wait nothing is sent. A request they had cancelled goes
		// back on their list, as it would if it had only been waiting; the
		// time of the decline is not touched.
		if cancelled {
			if _, err := s.pool.Exec(ctx,
				`UPDATE synastry_consents SET status = 'declined'
				 WHERE user_lo = $1 AND user_hi = $2 AND status = 'withdrawn' AND requested_by = $3`,
				lo, hi, me); err != nil {
				return "", err
			}
		}
		return answer, nil
	}
	// A fresh request reopens the question: from the member who declined
	// whenever they like, from the member who was declined once the wait is
	// over. Both conditions are checked again in the statement itself, so a
	// change between the read above and this write cannot skip the wait.
	tag, err := s.pool.Exec(ctx,
		`UPDATE synastry_consents
		 SET status = 'pending', requested_by = $3, created_at = NOW(), responded_at = NULL
		 WHERE user_lo = $1 AND user_hi = $2 AND status IN ('declined', 'withdrawn')
		   AND (requested_by <> $3 OR responded_at IS NULL
		        OR responded_at <= NOW() - make_interval(secs => $4))`,
		lo, hi, me, friendRequestCooldown.Seconds())
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "already-requested", nil
	}
	return answer, nil
}

// RespondSynastry answers a pending request made by the OTHER user. A decline
// records when it was given (responded_at): the wait before the request can
// be made again runs from there.
func (s *Store) RespondSynastry(ctx context.Context, me, other string, accept bool) (bool, error) {
	lo, hi := pairLoHi(me, other)
	status := "declined"
	if accept {
		status = "accepted"
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE synastry_consents SET status = $3, responded_at = NOW()
		 WHERE user_lo = $1 AND user_hi = $2 AND status = 'pending' AND requested_by <> $4`,
		lo, hi, status, me)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RevokeSynastry ends the pair's consent or request from the caller's side —
// either side, any time. The row is deleted, with one exception: a request the
// caller made and the other declined. Deleting that would let the caller ask
// again at once, so it is kept and only marked as withdrawn. The caller is
// answered, and from then on shown, exactly what cancelling a waiting request
// gives: done, and no request.
func (s *Store) RevokeSynastry(ctx context.Context, me, other string) (bool, error) {
	lo, hi := pairLoHi(me, other)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Read under a row lock, so that an answer being given at this moment is
	// waited for and the decision is made on the row as it really is.
	var status, requestedBy string
	err = tx.QueryRow(ctx,
		`SELECT status, requested_by FROM synastry_consents
		 WHERE user_lo = $1 AND user_hi = $2
		 FOR UPDATE`, lo, hi).Scan(&status, &requestedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if requestedBy == me {
		switch status {
		case "declined":
			if _, err := tx.Exec(ctx,
				`UPDATE synastry_consents SET status = 'withdrawn' WHERE user_lo = $1 AND user_hi = $2`,
				lo, hi); err != nil {
				return false, err
			}
			if err := tx.Commit(ctx); err != nil {
				return false, err
			}
			return true, nil
		case "withdrawn":
			// Already cancelled: to the caller there is nothing here.
			return false, nil
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM synastry_consents WHERE user_lo = $1 AND user_hi = $2`, lo, hi); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// ── shared status (self-published daily score, JSONB, opaque) ────────────────

func (s *Store) SharedStatus(ctx context.Context, userID string) (json.RawMessage, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT shared_status FROM users WHERE id = $1`, userID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return json.RawMessage(`null`), nil
	}
	return json.RawMessage(raw), nil
}

func (s *Store) SetSharedStatus(ctx context.Context, userID string, status json.RawMessage) error {
	if status == nil {
		_, err := s.pool.Exec(ctx, `UPDATE users SET shared_status = NULL WHERE id = $1`, userID)
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE users SET shared_status = $2 WHERE id = $1`, userID, status)
	return err
}

// ── nick (public display name; emails never leave the server) ────────────────

var ErrNickTaken = errors.New("store: nick taken")

// displayName: the chosen nick, or a stable anonymous handle from the id.
func displayName(nick *string, userID string) string {
	if nick != nil && *nick != "" {
		return *nick
	}
	tail := userID
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	return "seeker-" + tail
}

func (s *Store) Nick(ctx context.Context, userID string) (string, string, error) {
	var nick *string
	err := s.pool.QueryRow(ctx, `SELECT nick FROM users WHERE id = $1`, userID).Scan(&nick)
	if err != nil {
		return "", "", err
	}
	raw := ""
	if nick != nil {
		raw = *nick
	}
	return raw, displayName(nick, userID), nil
}

func (s *Store) SetNick(ctx context.Context, userID, nick string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET nick = $2 WHERE id = $1`, userID, nick)
	if err != nil && strings.Contains(err.Error(), "23505") {
		return ErrNickTaken
	}
	return err
}

// DisplayNameOf is the read used by handlers that previously exposed email.
func (s *Store) DisplayNameOf(ctx context.Context, userID string) (string, error) {
	var nick *string
	err := s.pool.QueryRow(ctx, `SELECT nick FROM users WHERE id = $1`, userID).Scan(&nick)
	if err != nil {
		return "", err
	}
	return displayName(nick, userID), nil
}

// ── avatar (small thumbnail, BYTEA, served inline as a data URL) ─────────────

// MaxAvatarBytes is the one cap on a profile picture, in decoded bytes.
//
// It is enforced twice. On upload, a larger picture is refused. On read, a
// picture already stored that is larger (uploaded under the old 200KB limit)
// is treated as absent wherever one member's picture is sent to ANOTHER
// member — friends list, requests, wall, Top 8, a friend's profile. Those
// responses carry many pictures inline, and a handful of old large ones was
// enough to push a list past the size a reply may have. The size test runs in
// SQL, so an oversized picture is never read out of the table for them. Only
// the owner's own read (Avatar) still returns it, so they can see and replace
// it.
const MaxAvatarBytes = 32 * 1024

var ErrAvatarTooLarge = errors.New("store: avatar exceeds MaxAvatarBytes")

func avatarDataURL(b []byte, mime *string) string {
	if len(b) == 0 || mime == nil || *mime == "" {
		return ""
	}
	return "data:" + *mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}

// Avatar is the OWNER's read of their own picture: whatever is stored, even
// one over MaxAvatarBytes. Never use it to show one member's picture to
// another; that is AvatarForOthers.
func (s *Store) Avatar(ctx context.Context, userID string) (string, error) {
	var b []byte
	var mime *string
	err := s.pool.QueryRow(ctx,
		`SELECT avatar, avatar_mime FROM users WHERE id = $1`, userID).Scan(&b, &mime)
	if err != nil {
		return "", err
	}
	return avatarDataURL(b, mime), nil
}

// AvatarForOthers is a member's picture as other members may receive it:
// empty when there is none or when the stored one is over MaxAvatarBytes.
func (s *Store) AvatarForOthers(ctx context.Context, userID string) (string, error) {
	var b []byte
	var mime *string
	err := s.pool.QueryRow(ctx,
		`SELECT CASE WHEN octet_length(avatar) <= $2 THEN avatar END, avatar_mime
		 FROM users WHERE id = $1`, userID, MaxAvatarBytes).Scan(&b, &mime)
	if err != nil {
		return "", err
	}
	return avatarDataURL(b, mime), nil
}

// SetAvatar stores a picture the handler has already validated. The size is
// checked again here so that no caller can store one over the cap.
func (s *Store) SetAvatar(ctx context.Context, userID string, img []byte, mime string) error {
	if len(img) > MaxAvatarBytes {
		return ErrAvatarTooLarge
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET avatar = $2, avatar_mime = $3 WHERE id = $1`, userID, img, mime)
	return err
}

func (s *Store) ClearAvatar(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET avatar = NULL, avatar_mime = NULL WHERE id = $1`, userID)
	return err
}

// ── profile wall (friends write, owner or author deletes) ────────────────────

type WallComment struct {
	ID           string `json:"id"`
	AuthorID     string `json:"authorId"`
	AuthorNick   string `json:"authorNick"`
	AuthorAvatar string `json:"authorAvatar,omitempty"`
	Body         string `json:"body"`
	CreatedAt    string `json:"createdAt"`
}

// Limits on what one member may write on one wall. A wall belongs to its
// owner; without them a single friend could fill it, and the owner's unread
// count with it.
const (
	// maxWallCommentsPerHour is how many comments one author may add to one
	// wall within any hour.
	maxWallCommentsPerHour = 20
	// maxWallCommentsStored is how many comments by one author one wall holds.
	maxWallCommentsStored = 200
)

var (
	// ErrNotFriends: the author is not an accepted friend of the wall's owner.
	ErrNotFriends = errors.New("store: not friends")
	// ErrWallTooFast: maxWallCommentsPerHour reached; room comes back with time.
	ErrWallTooFast = errors.New("store: too many comments on this wall in the last hour")
	// ErrWallFull: maxWallCommentsStored reached; room comes back by deleting.
	ErrWallFull = errors.New("store: too many comments by this author on this wall")
)

// AddWallComment stores a comment by authorID on profileID's wall.
//
// The friendship is checked here, in the transaction that inserts, under the
// pair's lock (the one RemoveFriend holds while it deletes the pair's
// comments). A check made earlier by the caller cannot do that: a comment
// could pass it, the friendship end, and the comment still land on the wall
// afterwards. The same lock makes two comments from one author count one after
// the other, so the limits hold for simultaneous requests too.
func (s *Store) AddWallComment(ctx context.Context, profileID, authorID, body string) (string, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, friendPairLockKey(profileID, authorID)); err != nil {
		return "", err
	}
	var friends bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM friendships
		                WHERE ((requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1))
		                  AND status = 'accepted')`,
		profileID, authorID).Scan(&friends)
	if err != nil {
		return "", err
	}
	if !friends {
		return "", ErrNotFriends
	}
	// The author's newest comments on this wall, at most as many as the wall
	// may hold from them: enough to answer both questions.
	var stored, lastHour int
	err = tx.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE c.created_at > NOW() - INTERVAL '1 hour')
		 FROM (SELECT created_at FROM profile_comments
		       WHERE profile_id = $1 AND author_id = $2
		       ORDER BY created_at DESC LIMIT $3) c`,
		profileID, authorID, maxWallCommentsStored).Scan(&stored, &lastHour)
	if err != nil {
		return "", err
	}
	if stored >= maxWallCommentsStored {
		return "", ErrWallFull
	}
	if lastHour >= maxWallCommentsPerHour {
		return "", ErrWallTooFast
	}
	id := newID("wc")
	if _, err := tx.Exec(ctx,
		`INSERT INTO profile_comments (id, profile_id, author_id, body) VALUES ($1, $2, $3, $4)`,
		id, profileID, authorID, body); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// Wall lists the newest comments on a profile.
//
// Only comments by members who are accepted friends of the wall's owner right
// now are listed (and the owner's own, should there be any). Ending a
// friendship deletes the pair's comments, so this matters for what was left
// behind before that was so: each row is shown with its author's current nick
// and picture, which nobody but a friend should be following.
func (s *Store) Wall(ctx context.Context, profileID string, limit int) ([]WallComment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT c.id, c.author_id, u.nick,
		        CASE WHEN octet_length(u.avatar) <= $3 THEN u.avatar END, u.avatar_mime,
		        c.body, c.created_at::text
		 FROM profile_comments c JOIN users u ON u.id = c.author_id
		 WHERE c.profile_id = $1
		   AND (c.author_id = c.profile_id OR EXISTS (
		        SELECT 1 FROM friendships f
		        WHERE f.status = 'accepted'
		          AND ((f.requester_id = c.profile_id AND f.addressee_id = c.author_id)
		            OR (f.requester_id = c.author_id AND f.addressee_id = c.profile_id))))
		 ORDER BY c.created_at DESC LIMIT $2`, profileID, limit, MaxAvatarBytes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WallComment{}
	for rows.Next() {
		var w WallComment
		var nick *string
		var ab []byte
		var am *string
		if err := rows.Scan(&w.ID, &w.AuthorID, &nick, &ab, &am, &w.Body, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.AuthorNick = displayName(nick, w.AuthorID)
		w.AuthorAvatar = avatarDataURL(ab, am)
		out = append(out, w)
	}
	// A read that fails part-way (timeout, dropped connection) ends the loop
	// just like the last row does; without this check the caller would get a
	// short wall reported as a complete one.
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteWallComment removes a comment if the caller owns the wall or wrote it.
func (s *Store) DeleteWallComment(ctx context.Context, commentID, byUserID string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM profile_comments WHERE id = $1 AND (profile_id = $2 OR author_id = $2)`,
		commentID, byUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UsersBrief resolves ids to public display rows (nick + avatar), preserving
// the input order — but only the ids that are accepted friends of ownerID.
//
// Used for Top 8. The owner curates the list, yet a stored id is only a string
// the owner once sent: resolving it unconditionally handed any member's nick
// and avatar to the owner's friends. An id is therefore resolved only while
// the accepted friendship that justifies showing that person still exists;
// anyone else (never a friend, pending, declined, since unfriended) is simply
// absent from the result.
func (s *Store) UsersBrief(ctx context.Context, ownerID string, ids []string) ([]FriendEntry, error) {
	if len(ids) == 0 {
		return []FriendEntry{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.nick,
		       CASE WHEN octet_length(u.avatar) <= $3 THEN u.avatar END, u.avatar_mime
		FROM users u
		JOIN friendships f ON f.status = 'accepted' AND (
		     (f.requester_id = $1 AND f.addressee_id = u.id) OR
		     (f.addressee_id = $1 AND f.requester_id = u.id))
		WHERE u.id = ANY($2)`, ownerID, ids, MaxAvatarBytes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]FriendEntry{}
	for rows.Next() {
		var e FriendEntry
		var nick *string
		var ab []byte
		var am *string
		if err := rows.Scan(&e.UserID, &nick, &ab, &am); err != nil {
			return nil, err
		}
		e.Nick = displayName(nick, e.UserID)
		e.Avatar = avatarDataURL(ab, am)
		byID[e.UserID] = e
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []FriendEntry{}
	for _, id := range ids {
		if e, ok := byID[id]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// AcceptedFriendIDs reports which of ids are accepted friends of ownerID.
// The owner's own id is never one of them.
func (s *Store) AcceptedFriendIDs(ctx context.Context, ownerID string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT CASE WHEN requester_id = $1 THEN addressee_id ELSE requester_id END
		FROM friendships
		WHERE status = 'accepted' AND (
		      (requester_id = $1 AND addressee_id = ANY($2)) OR
		      (addressee_id = $1 AND requester_id = ANY($2)))`, ownerID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ── discovery by nick ────────────────────────────────────────────────────────

// ErrNoSuchUser is UserByNick's answer when no member goes by the handle. Any
// other error from it means the lookup itself failed.
var ErrNoSuchUser = errors.New("store: no member goes by that nick")

// UserByNick resolves a public handle. Case-insensitive, matching the unique
// index from migration 000006.
func (s *Store) UserByNick(ctx context.Context, nick string) (*PublicUser, error) {
	n := strings.TrimSpace(nick)
	var u PublicUser
	err := s.pool.QueryRow(ctx,
		`SELECT id, email FROM users WHERE LOWER(nick) = LOWER($1)`, n).Scan(&u.ID, &u.Email)
	if err == nil {
		return &u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// Users who never chose one are shown as "seeker-<last 4 of id>", so that
	// handle has to resolve too — otherwise the app displays a name that
	// cannot be searched.
	low := strings.ToLower(n)
	if !strings.HasPrefix(low, "seeker-") || len(low) != len("seeker-")+4 {
		return nil, ErrNoSuchUser
	}
	//
	// Four characters of an id are not unique. When more than one member
	// without a nick ends in them, the handle names nobody: picking one (the
	// query used to take whichever row came first) sent a request meant for
	// one member to another. Two rows are enough to know.
	rows, err := s.pool.Query(ctx,
		`SELECT id, email FROM users
		 WHERE nick IS NULL AND LOWER(RIGHT(id, 4)) = $1
		 LIMIT 2`, low[len("seeker-"):])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	matches := 0
	for rows.Next() {
		if err := rows.Scan(&u.ID, &u.Email); err != nil {
			return nil, err
		}
		matches++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if matches != 1 {
		return nil, ErrNoSuchUser
	}
	return &u, nil
}

// ── unread marks ─────────────────────────────────────────────────────────────

// Unread counts what has arrived since the user last looked at each surface.
type Unread struct {
	FriendRequests   int `json:"friendRequests"`
	SynastryRequests int `json:"synastryRequests"`
	WallComments     int `json:"wallComments"`
	Total            int `json:"total"`
}

// UnreadCounts is deliberately time-based rather than a per-item read flag:
// every event already carries a timestamp, so one row per user answers all
// three questions without a join table per surface.
func (s *Store) UnreadCounts(ctx context.Context, userID string) (*Unread, error) {
	var friendsSeen, synastrySeen, wallSeen *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT friends_seen_at, synastry_seen_at, wall_seen_at
		 FROM social_seen WHERE user_id = $1`, userID).Scan(&friendsSeen, &synastrySeen, &wallSeen)
	if err != nil {
		// No row yet — never looked at anything.
		friendsSeen, synastrySeen, wallSeen = nil, nil, nil
	}

	out := &Unread{}
	// Incoming friend requests still pending.
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM friendships
		 WHERE addressee_id = $1 AND status = 'pending'
		   AND ($2::timestamptz IS NULL OR created_at > $2)`,
		userID, friendsSeen).Scan(&out.FriendRequests); err != nil {
		return nil, err
	}
	// Synastry requests awaiting THIS user's answer.
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synastry_consents
		 WHERE (user_lo = $1 OR user_hi = $1) AND status = 'pending' AND requested_by <> $1
		   AND ($2::timestamptz IS NULL OR created_at > $2)`,
		userID, synastrySeen).Scan(&out.SynastryRequests); err != nil {
		return nil, err
	}
	// Comments others left on this user's wall.
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM profile_comments
		 WHERE profile_id = $1 AND author_id <> $1
		   AND ($2::timestamptz IS NULL OR created_at > $2)`,
		userID, wallSeen).Scan(&out.WallComments); err != nil {
		return nil, err
	}
	out.Total = out.FriendRequests + out.SynastryRequests + out.WallComments
	return out, nil
}

// MarkSeen stamps one surface (or all of them) as read, now.
func (s *Store) MarkSeen(ctx context.Context, userID, scope string) error {
	col := map[string]string{
		"friends":  "friends_seen_at",
		"synastry": "synastry_seen_at",
		"wall":     "wall_seen_at",
	}[scope]

	if scope == "all" {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO social_seen (user_id, friends_seen_at, synastry_seen_at, wall_seen_at, updated_at)
			VALUES ($1, NOW(), NOW(), NOW(), NOW())
			ON CONFLICT (user_id) DO UPDATE SET
				friends_seen_at = NOW(), synastry_seen_at = NOW(),
				wall_seen_at = NOW(), updated_at = NOW()`, userID)
		return err
	}
	if col == "" {
		return errors.New("store: unknown seen scope")
	}
	// Column name comes from the fixed map above, never from user input.
	_, err := s.pool.Exec(ctx, `
		INSERT INTO social_seen (user_id, `+col+`, updated_at)
		VALUES ($1, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET `+col+` = NOW(), updated_at = NOW()`, userID)
	return err
}
