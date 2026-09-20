package registry

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/donvito/modelserver/internal/database"
	"github.com/donvito/modelserver/internal/models"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func TestRegistryCRUD(t *testing.T) {
	ctx := context.Background()
	r := newTestRegistry(t)

	m := &models.Model{Name: "gemma", Runtime: models.RuntimeLlamaCpp, Task: models.TaskChat, ModelPath: "/m/gemma.gguf"}
	if err := r.Create(ctx, m); err != nil {
		t.Fatal(err)
	}
	if m.ID == "" || m.Status != models.StatusStopped || string(m.Config) != "{}" {
		t.Fatalf("create did not fill defaults: %+v", m)
	}

	dup := &models.Model{Name: "gemma", Runtime: models.RuntimeONNX, ModelPath: "/x"}
	if err := r.Create(ctx, dup); !errors.Is(err, models.ErrNameTaken) {
		t.Fatalf("expected ErrNameTaken, got %v", err)
	}

	byID, err := r.Get(ctx, m.ID)
	if err != nil || byID.Name != "gemma" {
		t.Fatalf("get by id: %v %+v", err, byID)
	}
	byName, err := r.Resolve(ctx, "gemma")
	if err != nil || byName.ID != m.ID {
		t.Fatalf("resolve by name: %v", err)
	}
	if _, err := r.Resolve(ctx, "missing"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	m.Description = "desc"
	m.Config = []byte(`{"context_length":4096}`)
	if err := r.Update(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := r.SetStatus(ctx, m.ID, models.StatusRunning); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, m.ID)
	if got.Description != "desc" || got.Status != models.StatusRunning || string(got.Config) != `{"context_length":4096}` {
		t.Fatalf("update not persisted: %+v", got)
	}

	list, err := r.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}

	if err := r.Delete(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, m.ID); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("second delete should be not found, got %v", err)
	}
}

func TestRegistryValidation(t *testing.T) {
	r := newTestRegistry(t)
	bad := []*models.Model{
		{Name: "", Runtime: models.RuntimeLlamaCpp, ModelPath: "/x"},
		{Name: "ok", Runtime: "vllm", ModelPath: "/x"},
		{Name: "ok", Runtime: models.RuntimeLlamaCpp, Task: "juggling", ModelPath: "/x"},
		{Name: "ok", Runtime: models.RuntimeLlamaCpp, ModelPath: ""},
		{Name: "ok", Runtime: models.RuntimeLlamaCpp, ModelPath: "/x", Config: []byte("{nope")},
	}
	for i, m := range bad {
		if err := r.Create(context.Background(), m); !errors.Is(err, models.ErrValidation) {
			t.Errorf("case %d: expected ErrValidation, got %v", i, err)
		}
	}
}
