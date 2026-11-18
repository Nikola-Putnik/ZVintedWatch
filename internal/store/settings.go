package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// GetSetting decodes the JSON value stored under key into dst. It returns
// false (and leaves dst untouched) when the key does not exist.
func (s *Store) GetSetting(ctx context.Context, key string, dst any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), dst)
}

// SetSetting stores value (encoded as JSON) under key, replacing any previous value.
func (s *Store) SetSetting(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		key, string(raw))
	return err
}
