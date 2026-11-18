package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

const alertColumns = `id, provider, name, enabled, search_input, domain, title_filter, description_filter,
	price_min, price_max, interval_min_sec, interval_max_sec, destination_ids, created_at, updated_at`

// ListAlerts returns alerts ordered by name. An empty provider returns all.
func (s *Store) ListAlerts(ctx context.Context, provider string) ([]model.Alert, error) {
	query := `SELECT ` + alertColumns + ` FROM alerts`
	var args []any
	if provider != "" {
		query += ` WHERE provider = ?`
		args = append(args, provider)
	}
	query += ` ORDER BY name COLLATE NOCASE, id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := []model.Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// GetAlert returns one alert or ErrNotFound.
func (s *Store) GetAlert(ctx context.Context, id int64) (model.Alert, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+alertColumns+` FROM alerts WHERE id = ?`, id)
	a, err := scanAlert(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Alert{}, ErrNotFound
	}
	return a, err
}

// CreateAlert inserts a new alert and returns it with its id and timestamps.
func (s *Store) CreateAlert(ctx context.Context, a model.Alert) (model.Alert, error) {
	now := time.Now()
	a.CreatedAt, a.UpdatedAt = now, now
	cols, err := alertValues(a)
	if err != nil {
		return a, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO alerts (provider, name, enabled, search_input, domain,
		title_filter, description_filter, price_min, price_max, interval_min_sec, interval_max_sec,
		destination_ids, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, cols...)
	if err != nil {
		return a, fmt.Errorf("insert alert: %w", err)
	}
	a.ID, err = res.LastInsertId()
	return a, err
}

// UpdateAlert replaces every editable field of an existing alert.
func (s *Store) UpdateAlert(ctx context.Context, a model.Alert) (model.Alert, error) {
	existing, err := s.GetAlert(ctx, a.ID)
	if err != nil {
		return a, err
	}
	a.CreatedAt = existing.CreatedAt
	a.UpdatedAt = time.Now()
	cols, err := alertValues(a)
	if err != nil {
		return a, err
	}
	cols = append(cols, a.ID)
	_, err = s.db.ExecContext(ctx, `UPDATE alerts SET provider = ?, name = ?, enabled = ?, search_input = ?,
		domain = ?, title_filter = ?, description_filter = ?, price_min = ?, price_max = ?,
		interval_min_sec = ?, interval_max_sec = ?, destination_ids = ?, created_at = ?, updated_at = ?
		WHERE id = ?`, cols...)
	if err != nil {
		return a, fmt.Errorf("update alert: %w", err)
	}
	return a, nil
}

// SetAlertEnabled toggles an alert without touching its other fields.
func (s *Store) SetAlertEnabled(ctx context.Context, id int64, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE alerts SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolInt(enabled), toMillis(time.Now()), id)
	if err != nil {
		return err
	}
	return requireAffected(res)
}

// DeleteAlert removes an alert. Its match history is kept.
func (s *Store) DeleteAlert(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM alerts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(res)
}

// alertValues returns the column values in insert order (without id).
func alertValues(a model.Alert) ([]any, error) {
	title, err := json.Marshal(a.Title)
	if err != nil {
		return nil, err
	}
	desc, err := json.Marshal(a.Description)
	if err != nil {
		return nil, err
	}
	if a.DestinationIDs == nil {
		a.DestinationIDs = []int64{}
	}
	dests, err := json.Marshal(a.DestinationIDs)
	if err != nil {
		return nil, err
	}
	return []any{a.Provider, a.Name, boolInt(a.Enabled), a.SearchInput, a.Domain, string(title), string(desc),
		nullFloat(a.PriceMin), nullFloat(a.PriceMax), a.IntervalMinSec, a.IntervalMaxSec, string(dests),
		toMillis(a.CreatedAt), toMillis(a.UpdatedAt)}, nil
}

// scanner abstracts *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanAlert(sc scanner) (model.Alert, error) {
	var (
		a                    model.Alert
		enabled              int
		title, desc, dests   string
		priceMin, priceMax   sql.NullFloat64
		createdAt, updatedAt int64
	)
	if err := sc.Scan(&a.ID, &a.Provider, &a.Name, &enabled, &a.SearchInput, &a.Domain, &title, &desc,
		&priceMin, &priceMax, &a.IntervalMinSec, &a.IntervalMaxSec, &dests, &createdAt, &updatedAt); err != nil {
		return a, err
	}
	a.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(title), &a.Title); err != nil {
		return a, fmt.Errorf("alert %d title filter: %w", a.ID, err)
	}
	if err := json.Unmarshal([]byte(desc), &a.Description); err != nil {
		return a, fmt.Errorf("alert %d description filter: %w", a.ID, err)
	}
	if err := json.Unmarshal([]byte(dests), &a.DestinationIDs); err != nil {
		return a, fmt.Errorf("alert %d destinations: %w", a.ID, err)
	}
	if priceMin.Valid {
		a.PriceMin = &priceMin.Float64
	}
	if priceMax.Valid {
		a.PriceMax = &priceMax.Float64
	}
	a.CreatedAt, a.UpdatedAt = fromMillis(createdAt), fromMillis(updatedAt)
	normalizeFilter(&a.Title)
	normalizeFilter(&a.Description)
	return a, nil
}

// normalizeFilter replaces nil slices and empty modes with their defaults so
// API consumers always receive a complete structure.
func normalizeFilter(f *model.TextFilter) {
	if f.Include == nil {
		f.Include = []string{}
	}
	if f.Exclude == nil {
		f.Exclude = []string{}
	}
	if f.IncludeMode == "" {
		f.IncludeMode = model.MatchAll
	}
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func requireAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
