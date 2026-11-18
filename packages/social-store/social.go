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
	"strings"
	"time"
)

var ErrSelfFriend = errors.New("store: cannot befriend yourself")

// ── users ────────────────────────────────────────────────────────────────────

type PublicUser struct {
	ID    string `json:"userId"`
	Email string `json:"email"`
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*PublicUser, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, email FROM users WHERE email = $1`, strings.ToLower(strings.TrimSpace(email)))
	var u PublicUser
	if err := row.Scan(&u.ID, &u.Email); err != nil {
		return nil, err
	}
	return &u, nil
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
	ds := *dateStr
	if len(ds) > 10 {
		ds = ds[:10] // birth_date may be a DATE or a timestamp::text
	}
	var y, m, d int
	if _, err := fmt.Sscanf(ds, "%d-%d-%d", &y, &m, &d); err != nil {
		return nil, fmt.Errorf("bad birth_date %q", ds)
	}
	hh, mm := 12, 0
	if timeStr != nil && *timeStr != "" {
		ts := *timeStr
		if len(ts) >= 5 {
			fmt.Sscanf(ts[:5], "%d:%d", &hh, &mm)
		}
	}
	off := 0.0
	if tzOff != nil {
		off = *tzOff
		// legacy rows stored minutes; the TS server normalizes the same way
		if off > 14 || off < -14 {
			off = -(off / 60)
		}
	}
	utc := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC).
		Add(time.Duration(float64(hh)-off) * time.Hour).
		Add(time.Duration(mm) * time.Minute)
	bm := &BirthMoment{UTC: utc}
	if lat != nil {
		bm.Latitude = *lat
	}
	if lon != nil {
		bm.Longitude = *lon
	}
	return bm, nil
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

type FriendRequestEntry struct {
	RequestID string `json:"requestId"`
	UserID    string `json:"userId"`
	Nick      string `json:"nick"`
	Avatar    string `json:"avatar,omitempty"`
}

type Friendships struct {
	Accepted []FriendEntry        `json:"accepted"`
	Incoming []FriendRequestEntry `json:"incoming"`
	Outgoing []FriendRequestEntry `json:"outgoing"`
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(b))
}

// CreateFriendRequest inserts one pending edge unless a row already exists in
// either direction. Returns "created" | "exists".
func (s *Store) CreateFriendRequest(ctx context.Context, requesterID, addresseeID string) (string, error) {
	if requesterID == addresseeID {
		return "", ErrSelfFriend
	}
	var one int
	err := s.pool.QueryRow(ctx,
		`SELECT 1 FROM friendships
		 WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)`,
		requesterID, addresseeID).Scan(&one)
	if err == nil {
		return "exists", nil
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO friendships (id, requester_id, addressee_id, status) VALUES ($1, $2, $3, 'pending')`,
		newID("fr"), requesterID, addresseeID)
	if err != nil {
		return "", err
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
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
// Outgoing requests intentionally omit avatars, matching the existing API.
func (s *Store) ListFriendships(ctx context.Context, userID string) (*Friendships, error) {
	out := &Friendships{Accepted: []FriendEntry{}, Incoming: []FriendRequestEntry{}, Outgoing: []FriendRequestEntry{}}
	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.requester_id, f.status, u.id, u.nick,
		       COALESCE(f.responded_at::text, ''),
		       CASE WHEN f.status = 'accepted' OR f.addressee_id = $1 THEN u.avatar END,
		       CASE WHEN f.status = 'accepted' OR f.addressee_id = $1 THEN u.avatar_mime END,
		       COALESCE(c.status, 'none'), COALESCE(c.requested_by, '')
		FROM friendships f
		JOIN users u ON u.id = CASE WHEN f.requester_id = $1 THEN f.addressee_id ELSE f.requester_id END
		LEFT JOIN synastry_consents c ON f.status = 'accepted' AND
		    ((c.user_lo = $1 AND c.user_hi = u.id) OR (c.user_hi = $1 AND c.user_lo = u.id))
		WHERE (f.requester_id = $1 OR f.addressee_id = $1) AND f.status IN ('accepted', 'pending')`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var requestID, requesterID, status, otherID, since, consent, requestedBy string
		var nick, mime *string
		var avatar []byte
		if err := rows.Scan(&requestID, &requesterID, &status, &otherID, &nick, &since,
			&avatar, &mime, &consent, &requestedBy); err != nil {
			return nil, err
		}
		name := displayName(nick, otherID)
		picture := avatarDataURL(avatar, mime)
		if status == "accepted" {
			switch consent {
			case "pending":
				if requestedBy == userID {
					consent = "pending-out"
				} else {
					consent = "pending-in"
				}
			case "declined":
				// A decline stays hidden from the requester.
				if requestedBy == userID {
					consent = "none"
				}
			}
			out.Accepted = append(out.Accepted, FriendEntry{UserID: otherID, Nick: name, Since: since, Synastry: consent, Avatar: picture})
		} else {
			e := FriendRequestEntry{RequestID: requestID, UserID: otherID, Nick: name, Avatar: picture}
			if requesterID == userID {
				out.Outgoing = append(out.Outgoing, e)
			} else {
				out.Incoming = append(out.Incoming, e)
			}
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

func pairLoHi(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

type SynastryConsent struct {
	Status      string // none | pending | accepted | declined
	RequestedBy string // empty when Status == none
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
func (s *Store) RequestSynastry(ctx context.Context, me, other string) (string, error) {
	lo, hi := pairLoHi(me, other)
	cur, err := s.SynastryConsentState(ctx, me, other)
	if err != nil {
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
	default: // declined — a fresh request reopens the question (status-guarded)
		tag, err := s.pool.Exec(ctx,
			`UPDATE synastry_consents
			 SET status = 'pending', requested_by = $3, created_at = NOW(), responded_at = NULL
			 WHERE user_lo = $1 AND user_hi = $2 AND status = 'declined'`, lo, hi, me)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() == 0 {
			return "already-requested", nil
		}
		return "requested", nil
	}
}

// RespondSynastry answers a pending request made by the OTHER user.
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

// RevokeSynastry deletes the pair's consent row — either side, any time.
func (s *Store) RevokeSynastry(ctx context.Context, me, other string) (bool, error) {
	lo, hi := pairLoHi(me, other)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM synastry_consents WHERE user_lo = $1 AND user_hi = $2`, lo, hi)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
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

func avatarDataURL(b []byte, mime *string) string {
	if len(b) == 0 || mime == nil || *mime == "" {
		return ""
	}
	return "data:" + *mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}

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

func (s *Store) SetAvatar(ctx context.Context, userID string, img []byte, mime string) error {
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

func (s *Store) AddWallComment(ctx context.Context, profileID, authorID, body string) (string, error) {
	id := newID("wc")
	_, err := s.pool.Exec(ctx,
		`INSERT INTO profile_comments (id, profile_id, author_id, body) VALUES ($1, $2, $3, $4)`,
		id, profileID, authorID, body)
	return id, err
}

func (s *Store) Wall(ctx context.Context, profileID string, limit int) ([]WallComment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT c.id, c.author_id, u.nick, u.avatar, u.avatar_mime, c.body, c.created_at::text
		 FROM profile_comments c JOIN users u ON u.id = c.author_id
		 WHERE c.profile_id = $1 ORDER BY c.created_at DESC LIMIT $2`, profileID, limit)
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

// UsersBrief resolves ids to public display rows (email + avatar), preserving
// the input order. Used for Top 8 — the owner curates who appears.
func (s *Store) UsersBrief(ctx context.Context, ids []string) ([]FriendEntry, error) {
	if len(ids) == 0 {
		return []FriendEntry{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, nick, avatar, avatar_mime FROM users WHERE id = ANY($1)`, ids)
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
	out := []FriendEntry{}
	for _, id := range ids {
		if e, ok := byID[id]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// ── discovery by nick ────────────────────────────────────────────────────────

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
	// Users who never chose one are shown as "seeker-<last 4 of id>", so that
	// handle has to resolve too — otherwise the app displays a name that
	// cannot be searched.
	if low := strings.ToLower(n); strings.HasPrefix(low, "seeker-") && len(low) == len("seeker-")+4 {
		tail := low[len("seeker-"):]
		err = s.pool.QueryRow(ctx,
			`SELECT id, email FROM users
			 WHERE nick IS NULL AND LOWER(RIGHT(id, 4)) = $1
			 LIMIT 1`, tail).Scan(&u.ID, &u.Email)
		if err == nil {
			return &u, nil
		}
	}
	return nil, err
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
