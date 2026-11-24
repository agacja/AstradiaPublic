package socialstore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryCounter struct{ count atomic.Int64 }

func (q *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.count.Add(1)
	return ctx
}
func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestFriendshipsOneQueryPreservesConsentAndAvatarVisibility(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a disposable local PostgreSQL database")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Port() == "5432" {
		t.Fatal("test database must use loopback and a nonstandard port")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	counter := &queryCounter{}
	cfg.ConnConfig.Tracer = counter
	cfg.MaxConns, cfg.MinConns = 1, 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// Session-local tables on one connection cannot modify existing accounts.
	_, err = pool.Exec(ctx, `
		CREATE TEMP TABLE users (id text PRIMARY KEY, nick text, avatar bytea, avatar_mime text);
		CREATE TEMP TABLE friendships (id text, requester_id text, addressee_id text, status text, responded_at timestamptz, created_at timestamptz DEFAULT now());
		CREATE TEMP TABLE synastry_consents (user_lo text, user_hi text, requested_by text, status text, UNIQUE(user_lo,user_hi));
		INSERT INTO users VALUES ('me', 'Me', NULL, NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	// "hidden-decline" is a request of mine that was declined: I am shown it
	// as still waiting. "cancelled-decline" is one I cancelled after that: I
	// am shown nothing. "their-cancelled" is one I declined and they then
	// cancelled: to me it is still the request I declined.
	for i, consent := range []string{"none", "accepted", "pending-out", "pending-in", "hidden-decline", "declined", "cancelled-decline", "their-cancelled"} {
		other := fmt.Sprintf("friend-%d", i)
		if _, err := pool.Exec(ctx, `INSERT INTO users VALUES ($1, $2, $3, 'image/png')`, other, other, []byte{1, 2, 3}); err != nil {
			t.Fatal(err)
		}
		requester, addressee := "me", other
		if i%2 == 0 {
			requester, addressee = other, "me"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO friendships VALUES ($1,$2,$3,'accepted',now())`, other, requester, addressee); err != nil {
			t.Fatal(err)
		}
		want[other] = consent
		if consent != "none" {
			status, requestedBy := consent, other
			switch consent {
			case "pending-out":
				status, requestedBy = "pending", "me"
			case "pending-in":
				status = "pending"
			case "hidden-decline":
				status, requestedBy, want[other] = "declined", "me", "pending-out"
			case "cancelled-decline":
				status, requestedBy, want[other] = "withdrawn", "me", "none"
			case "their-cancelled":
				status, want[other] = "withdrawn", "declined"
			}
			lo, hi := pairLoHi("me", other)
			if _, err := pool.Exec(ctx, `INSERT INTO synastry_consents VALUES ($1,$2,$3,$4)`, lo, hi, requestedBy, status); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users VALUES ('incoming',NULL,decode('010203','hex'),'image/png'),('outgoing','Outgoing',decode('010203','hex'),'image/png'),('stranger','Stranger',NULL,NULL);
		INSERT INTO friendships VALUES ('in','incoming','me','pending',NULL),('out','me','outgoing','pending',NULL),('hidden','stranger','me','declined',NULL),('unrelated','incoming','stranger','accepted',now());`)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{pool: pool}
	counter.count.Store(0)
	got, err := s.ListFriendships(ctx, "me")
	if err != nil {
		t.Fatal(err)
	}
	if n := counter.count.Load(); n != 1 {
		t.Fatalf("friend list required %d queries, want 1", n)
	}
	if len(got.Accepted) != len(want) || len(got.Incoming) != 1 || len(got.Outgoing) != 1 {
		t.Fatalf("unexpected relationship visibility: %+v", got)
	}
	for _, friend := range got.Accepted {
		if state, ok := want[friend.UserID]; !ok || friend.Synastry != state || friend.Since == "" || friend.Avatar != "data:image/png;base64,AQID" {
			t.Fatalf("changed friend visibility: %+v", friend)
		}
	}
	if got.Incoming[0].UserID != "incoming" || got.Incoming[0].RequestID != "in" || got.Incoming[0].Nick != displayName(nil, "incoming") {
		t.Fatalf("changed incoming request: %+v", got.Incoming)
	}
	if got.Outgoing[0].UserID != "outgoing" || got.Outgoing[0].RequestID != "out" || got.Outgoing[0].Nick != "Outgoing" {
		t.Fatalf("changed outgoing request: %+v", got.Outgoing)
	}
	// A request carries no picture in either direction, although both of
	// these members have one.
	requests, err := json.Marshal([]any{got.Incoming, got.Outgoing})
	if err != nil || strings.Contains(string(requests), "avatar") || strings.Contains(string(requests), "AQID") {
		t.Fatalf("a request exposes a picture: %s (%v)", requests, err)
	}
	empty, err := s.ListFriendships(ctx, "no-friends")
	if err != nil || empty.Accepted == nil || empty.Incoming == nil || empty.Outgoing == nil || len(empty.Accepted)+len(empty.Incoming)+len(empty.Outgoing) != 0 {
		t.Fatalf("empty lists must remain arrays: %+v, %v", empty, err)
	}
}
