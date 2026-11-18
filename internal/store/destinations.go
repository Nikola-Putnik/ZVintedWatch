package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

// ListDestinations returns every Telegram destination ordered by name.
func (s *Store) ListDestinations(ctx context.Context) ([]model.Destination, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, chat_id, created_at FROM destinations ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Destination{}
	for rows.Next() {
		var d model.Destination
		var created int64
		if err := rows.Scan(&d.ID, &d.Name, &d.ChatID, &created); err != nil {
			return nil, err
		}
		d.CreatedAt = fromMillis(created)
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDestination returns one destination or ErrNotFound.
func (s *Store) GetDestination(ctx context.Context, id int64) (model.Destination, error) {
	var d model.Destination
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT id, name, chat_id, created_at FROM destinations WHERE id = ?`, id).
		Scan(&d.ID, &d.Name, &d.ChatID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	d.CreatedAt = fromMillis(created)
	return d, err
}

// CreateDestination inserts a destination.
func (s *Store) CreateDestination(ctx context.Context, d model.Destination) (model.Destination, error) {
	d.CreatedAt = time.Now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO destinations (name, chat_id, created_at) VALUES (?, ?, ?)`,
		d.Name, d.ChatID, toMillis(d.CreatedAt))
	if err != nil {
		return d, err
	}
	d.ID, err = res.LastInsertId()
	return d, err
}

// UpdateDestination renames a destination or changes its chat id.
func (s *Store) UpdateDestination(ctx context.Context, d model.Destination) error {
	res, err := s.db.ExecContext(ctx, `UPDATE destinations SET name = ?, chat_id = ? WHERE id = ?`,
		d.Name, d.ChatID, d.ID)
	if err != nil {
		return err
	}
	return requireAffected(res)
}

// DeleteDestination removes a destination. Alerts keep the dangling id, which
// is ignored when sending notifications.
func (s *Store) DeleteDestination(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM destinations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(res)
}
