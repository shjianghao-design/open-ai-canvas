package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func TestCanvasWorkflowImportCreatesOwnedIndependentCanvas(t *testing.T) {
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: "file:" + newID() + "?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.CanvasProject{}, &model.CanvasWorkflowTemplate{}, &model.CanvasWorkflowVersion{}, &model.CanvasWorkflowImport{}, &model.User{}, &model.AdminAuditEvent{}); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	if err := db.Create(&model.User{ID: "customer", Username: "customer", CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repo, dataDir: t.TempDir()}
	templates, err := svc.CanvasWorkflowTemplates("customer")
	if err != nil || len(templates) == 0 {
		t.Fatalf("templates: %v, %d", err, len(templates))
	}
	if _, err := svc.CanvasWorkflowTemplates("customer"); err != nil {
		t.Fatalf("builtin registration is not idempotent: %v", err)
	}
	// 内置模板可以有多个，但同一 slug 只能有一条记录（幂等注册）。
	var templateCount, distinctSlugs int64
	if err := db.Model(&model.CanvasWorkflowTemplate{}).Count(&templateCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.CanvasWorkflowTemplate{}).Distinct("slug").Count(&distinctSlugs).Error; err != nil {
		t.Fatal(err)
	}
	if templateCount != distinctSlugs {
		t.Fatalf("duplicate builtin template rows: rows=%d distinctSlugs=%d", templateCount, distinctSlugs)
	}
	if templateCount < 1 {
		t.Fatal("expected at least one builtin template")
	}
	imported, err := svc.ImportCanvasWorkflowTemplate("customer", "ecommerce-product-detail-suite")
	if err != nil {
		t.Fatal(err)
	}
	if imported.ID == "" || imported.UserID != "customer" || imported.Revision != 1 {
		t.Fatalf("unexpected canvas: %#v", imported)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(imported.PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["id"] != imported.ID {
		t.Fatalf("document id %v != canvas id %s", payload["id"], imported.ID)
	}
	raw, _ := json.Marshal(payload)
	for _, forbidden := range []string{"taskId", "apiKey", "generationBatches"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("import contains %s: %s", forbidden, raw)
		}
	}
	nodes := payload["nodes"].([]any)
	if len(nodes) < 5 {
		t.Fatalf("expected product input and output nodes, got %d", len(nodes))
	}
	first := nodes[0].(map[string]any)
	second := nodes[1].(map[string]any)
	if first["id"] == second["id"] {
		t.Fatal("node IDs were not regenerated")
	}
	connections := payload["connections"].([]any)
	if len(connections) < 4 {
		t.Fatalf("expected input to output edges, got %d", len(connections))
	}
	secondImport, err := svc.ImportCanvasWorkflowTemplate("customer", "ecommerce-product-detail-suite")
	if err != nil {
		t.Fatal(err)
	}
	if secondImport.ID == imported.ID {
		t.Fatal("repeated import reused the same canvas")
	}
	stored, err := repo.CanvasProjectForUser("customer", imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PayloadJSON != imported.PayloadJSON {
		t.Fatal("second import overwrote the first canvas")
	}
}

func TestCanvasWorkflowImportRejectsInvalidUser(t *testing.T) {
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: "file:" + newID() + "?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repository.New(db)}
	if _, err := svc.ImportCanvasWorkflowTemplate("", "ecommerce-product-detail-suite"); err == nil {
		t.Fatal("expected authentication error")
	}
	if _, err := svc.ImportCanvasWorkflowTemplate("missing-user", "ecommerce-product-detail-suite"); err == nil {
		t.Fatal("expected invalid user error")
	}
}
