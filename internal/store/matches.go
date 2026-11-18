package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

// MatchFilter narrows ListMatches results. Zero values mean "no filter".
type MatchFilter struct {
	Provider string
	AlertID  int64
	Limit    int
}

// InsertMatch records a match. It returns false when the same alert already
// matched the same item (the match is then not recorded twice).
func (s *Store) InsertMatch(ctx context.Context, m *model.Match) (bool, error) {
	listing, err := json.Marshal(m.Listing)
	if err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO matches (alert_id, alert_name, provider, item_id, listing, matched_at, delivery)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		m.AlertID, m.AlertName, m.Provider, m.Listing.ID, string(listing), toMillis(m.MatchedAt), m.Delivery)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	m.ID, err = res.LastInsertId()
	return true, err
}

// SetMatchDelivery updates the delivery state of matches by id.
func (s *Store) SetMatchDelivery(ctx context.Context, ids []int64, delivery string) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `UPDATE matches SET delivery = ? WHERE id = ?`, delivery, id); err != nil {
			return err
		}
	}
	return nil
}

// ListMatches returns the most recent matches first.
func (s *Store) ListMatches(ctx context.Context, f MatchFilter) ([]model.Match, error) {
	query := `SELECT id, alert_id, alert_name, provider, listing, matched_at, delivery FROM matches WHERE 1 = 1`
	var args []any
	if f.Provider != "" {
		query += ` AND provider = ?`
		args = append(args, f.Provider)
	}
	if f.AlertID != 0 {
		query += ` AND alert_id = ?`
		args = append(args, f.AlertID)
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	query += ` ORDER BY matched_at DESC, id DESC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Match{}
	for rows.Next() {
		var m model.Match
		var listing string
		var at int64
		if err := rows.Scan(&m.ID, &m.AlertID, &m.AlertName, &m.Provider, &listing, &at, &m.Delivery); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(listing), &m.Listing); err != nil {
			return nil, err
		}
		m.MatchedAt = fromMillis(at)
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountMatchesSince counts matches per provider since the given time.
func (s *Store) CountMatchesSince(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, COUNT(*) FROM matches WHERE matched_at >= ? GROUP BY provider`, toMillis(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var p string
		var n int
		if err := rows.Scan(&p, &n); err != nil {
			return nil, err
		}
		out[p] = n
	}
	return out, rows.Err()
}

// ClaimDelivery records that an item is being sent to a destination. It
// returns false when the item was already delivered there, which prevents
// duplicate notifications across alerts and searches.
func (s *Store) ClaimDelivery(ctx context.Context, destinationID int64, provider, itemID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO deliveries (destination_id, provider, item_id, sent_at)
		VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`, destinationID, provider, itemID, toMillis(time.Now()))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ReleaseDelivery forgets a delivery claim, typically after a send failure so
// a later match can retry.
func (s *Store) ReleaseDelivery(ctx context.Context, destinationID int64, provider, itemID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM deliveries WHERE destination_id = ? AND provider = ? AND item_id = ?`,
		destinationID, provider, itemID)
	return err
}

// PruneResult reports how many rows Prune removed.
type PruneResult struct {
	SeenItems, CachedItems, Matches, Deliveries int64
}

// Prune deletes housekeeping rows older than the given retention periods.
// seenRetention also applies to the description cache and delivery claims.
func (s *Store) Prune(ctx context.Context, seenRetention, matchRetention time.Duration) (PruneResult, error) {
	var r PruneResult
	now := time.Now()
	steps := []struct {
		query string
		dst   *int64
		age   time.Duration
	}{
		{`DELETE FROM seen_items WHERE last_seen_at < ?`, &r.SeenItems, seenRetention},
		{`DELETE FROM item_cache WHERE fetched_at < ?`, &r.CachedItems, seenRetention},
		{`DELETE FROM deliveries WHERE sent_at < ?`, &r.Deliveries, seenRetention},
		{`DELETE FROM matches WHERE matched_at < ?`, &r.Matches, matchRetention},
	}
	for _, st := range steps {
		res, err := s.db.ExecContext(ctx, st.query, toMillis(now.Add(-st.age)))
		if err != nil {
			return r, err
		}
		*st.dst, _ = res.RowsAffected()
	}
	return r, nil
}
