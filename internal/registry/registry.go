// Package registry persists Model records in SQLite.
package registry

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/donvito/modelserver/internal/models"
)

type Registry struct {
	db *sql.DB
}

func New(db *sql.DB) *Registry { return &Registry{db: db} }

const columns = `id, name, description, runtime, task, model_path, config, status, created_at, updated_at`

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "mdl_" + hex.EncodeToString(b)
}

// Create inserts a new model, assigning ID and timestamps.
func (r *Registry) Create(ctx context.Context, m *models.Model) error {
	if err := m.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	m.ID = newID()
	m.CreatedAt, m.UpdatedAt = now, now
	if m.Status == "" {
		m.Status = models.StatusStopped
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO models(`+columns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.Name, m.Description, m.Runtime, m.Task, m.ModelPath, string(m.Config), m.Status, m.CreatedAt, m.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return models.ErrNameTaken
		}
		return fmt.Errorf("insert model: %w", err)
	}
	return nil
}

// Update persists all mutable fields of m.
func (r *Registry) Update(ctx context.Context, m *models.Model) error {
	if err := m.Validate(); err != nil {
		return err
	}
	m.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `UPDATE models SET name=?, description=?, runtime=?, task=?, model_path=?, config=?, status=?, updated_at=? WHERE id=?`,
		m.Name, m.Description, m.Runtime, m.Task, m.ModelPath, string(m.Config), m.Status, m.UpdatedAt, m.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return models.ErrNameTaken
		}
		return fmt.Errorf("update model: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ErrNotFound
	}
	return nil
}

// SetStatus records the last requested lifecycle state.
func (r *Registry) SetStatus(ctx context.Context, id, status string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE models SET status=?, updated_at=? WHERE id=?`, status, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ErrNotFound
	}
	return nil
}

func (r *Registry) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM models WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ErrNotFound
	}
	return nil
}

func (r *Registry) Get(ctx context.Context, id string) (*models.Model, error) {
	return r.one(ctx, `SELECT `+columns+` FROM models WHERE id=?`, id)
}

func (r *Registry) GetByName(ctx context.Context, name string) (*models.Model, error) {
	return r.one(ctx, `SELECT `+columns+` FROM models WHERE name=?`, name)
}

// Resolve accepts either an ID or a name.
func (r *Registry) Resolve(ctx context.Context, ref string) (*models.Model, error) {
	m, err := r.Get(ctx, ref)
	if errors.Is(err, models.ErrNotFound) {
		return r.GetByName(ctx, ref)
	}
	return m, err
}

func (r *Registry) List(ctx context.Context) ([]*models.Model, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+columns+` FROM models ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.Model
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if out == nil {
		out = []*models.Model{}
	}
	return out, rows.Err()
}

func (r *Registry) one(ctx context.Context, q string, args ...any) (*models.Model, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, models.ErrNotFound
	}
	return scan(rows)
}

type scanner interface {
	Scan(dest ...any) error
}

func scan(s scanner) (*models.Model, error) {
	var m models.Model
	var cfg string
	if err := s.Scan(&m.ID, &m.Name, &m.Description, &m.Runtime, &m.Task, &m.ModelPath, &cfg, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	if cfg == "" {
		cfg = "{}"
	}
	m.Config = json.RawMessage(cfg)
	return &m, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
