package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

const searchColumns = `id, provider, key, label, web_url, seeded, last_polled_at, last_status, last_error, created_at`

// EnsureSearch returns the search identified by key, creating it when needed.
// The label and web URL are refreshed on every call.
func (s *Store) EnsureSearch(ctx context.Context, provider, key, label, webURL string) (model.Search, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO searches (provider, key, label, web_url, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET label = excluded.label, web_url = excluded.web_url`,
		provider, key, label, webURL, toMillis(time.Now()))
	if err != nil {
		return model.Search{}, err
	}
	return s.getSearchBy(ctx, `key = ?`, key)
}

// GetSearch returns a search by id or ErrNotFound.
func (s *Store) GetSearch(ctx context.Context, id int64) (model.Search, error) {
	return s.getSearchBy(ctx, `id = ?`, id)
}

func (s *Store) getSearchBy(ctx context.Context, where string, arg any) (model.Search, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+searchColumns+` FROM searches WHERE `+where, arg)
	se, err := scanSearch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return se, ErrNotFound
	}
	return se, err
}

// ListSearches returns every persisted search.
func (s *Store) ListSearches(ctx context.Context) ([]model.Search, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+searchColumns+` FROM searches ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Search{}
	for rows.Next() {
		se, err := scanSearch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, se)
	}
	return out, rows.Err()
}

// UpdateSearchState records the outcome of a poll.
func (s *Store) UpdateSearchState(ctx context.Context, id int64, seeded bool, polledAt time.Time, status, lastErr string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE searches SET seeded = ?, last_polled_at = ?, last_status = ?, last_error = ?
		WHERE id = ?`, boolInt(seeded), toMillis(polledAt), status, lastErr, id)
	return err
}

// DeleteSearchesExcept removes the searches whose key is not in keep, together
// with their seen items. It is called when alerts change so that searches no
// alert uses anymore do not accumulate.
func (s *Store) DeleteSearchesExcept(ctx context.Context, keep []string) error {
	if len(keep) == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM searches`)
		return err
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keep)), ",")
	args := make([]any, len(keep))
	for i, k := range keep {
		args[i] = k
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM searches WHERE key NOT IN (`+placeholders+`)`, args...)
	return err
}

func scanSearch(sc scanner) (model.Search, error) {
	var se model.Search
	var seeded int
	var polled, created int64
	err := sc.Scan(&se.ID, &se.Provider, &se.Key, &se.Label, &se.WebURL, &seeded, &polled,
		&se.LastStatus, &se.LastError, &created)
	se.Seeded = seeded != 0
	se.LastPolledAt, se.CreatedAt = fromMillis(polled), fromMillis(created)
	return se, err
}

// MarkSeen records item ids for a search and returns the ids that had never
// been seen before, preserving the input order. Items seen again get their
// last_seen_at refreshed, so listings that stay in the results are never
// pruned and therefore never reported as new a second time.
func (s *Store) MarkSeen(ctx context.Context, searchID int64, itemIDs []string, at time.Time) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	insert, err := tx.PrepareContext(ctx, `INSERT INTO seen_items (search_id, item_id, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`)
	if err != nil {
		return nil, err
	}
	defer insert.Close()
	touch, err := tx.PrepareContext(ctx, `UPDATE seen_items SET last_seen_at = ? WHERE search_id = ? AND item_id = ?`)
	if err != nil {
		return nil, err
	}
	defer touch.Close()

	fresh := []string{}
	ts := toMillis(at)
	for _, id := range itemIDs {
		res, err := insert.ExecContext(ctx, searchID, id, ts, ts)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			fresh = append(fresh, id)
			continue
		}
		if _, err := touch.ExecContext(ctx, ts, searchID, id); err != nil {
			return nil, err
		}
	}
	return fresh, tx.Commit()
}

// GetCachedDescription returns a previously fetched listing description.
func (s *Store) GetCachedDescription(ctx context.Context, provider, itemID string) (string, bool, error) {
	var desc string
	err := s.db.QueryRowContext(ctx, `SELECT description FROM item_cache WHERE provider = ? AND item_id = ?`,
		provider, itemID).Scan(&desc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return desc, err == nil, err
}

// CacheDescription stores a listing description.
func (s *Store) CacheDescription(ctx context.Context, provider, itemID, description string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO item_cache (provider, item_id, description, fetched_at)
		VALUES (?, ?, ?, ?) ON CONFLICT (provider, item_id) DO UPDATE SET description = excluded.description,
		fetched_at = excluded.fetched_at`, provider, itemID, description, toMillis(time.Now()))
	return err
}
