package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func TestValidateCanvasWorkflowCoverRequiresCropped1200x675Image(t *testing.T) {
	makeHeader := func(content []byte) *multipart.FileHeader {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "cover.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("POST", "/", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		if err := request.ParseMultipartForm(CanvasWorkflowCoverMaxBytes + 1024); err != nil {
			t.Fatal(err)
		}
		return request.MultipartForm.File["file"][0]
	}
	encodePNG := func(width, height int) []byte {
		var out bytes.Buffer
		if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	t.Run("accepts exact output dimensions", func(t *testing.T) {
		if err := validateCanvasWorkflowCoverUpload(makeHeader(encodePNG(1200, 675))); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rejects wrong dimensions", func(t *testing.T) {
		if err := validateCanvasWorkflowCoverUpload(makeHeader(encodePNG(1200, 1200))); err == nil {
			t.Fatal("expected non-16:9 cover rejection")
		}
	})
	t.Run("rejects spoofed image content", func(t *testing.T) {
		if err := validateCanvasWorkflowCoverUpload(makeHeader([]byte("not an image"))); err == nil {
			t.Fatal("expected invalid image rejection")
		}
	})
}

func newWorkflowAdminFixture(t *testing.T) (*Service, *model.User, *model.CanvasProject) {
	t.Helper()
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: "file:" + newID() + "?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.CanvasProject{}, &model.CanvasWorkflowTemplate{}, &model.CanvasWorkflowVersion{},
		&model.CanvasWorkflowImport{}, &model.CanvasWorkflowCoverDraft{}, &model.Resource{}, &model.User{}, &model.AdminAuditEvent{},
	); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	admin := &model.User{ID: "admin-1", Username: "admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(admin).Error; err != nil {
		t.Fatal(err)
	}
	svc := &Service{repo: repo, dataDir: t.TempDir()}
	canvas := &model.CanvasProject{
		ID: "canvas-1", UserID: admin.ID, Title: "我的画布",
		PayloadJSON: `{"title":"我的画布","nodes":[{"id":"n1","type":"image","title":"商品图","position":{"x":0,"y":0},"width":380,"height":380,"metadata":{"content":"/workflow-samples/sample-1.jpg","status":"success"}}],"connections":[],"chatSessions":[],"activeChatId":null,"viewport":{"x":0,"y":0,"k":1}}`,
		Revision:    0, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.UpsertCanvasProject(canvas); err != nil {
		t.Fatal(err)
	}
	return svc, admin, canvas
}

func containsWorkflowMediaType(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestWorkflowDraftIsNotVisibleUntilPublished(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)

	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "my-template", Name: "我的模板", Category: "电商", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Published {
		t.Fatal("new template must not be published")
	}
	if detail.Revision != 1 {
		t.Fatalf("revision = %d, want 1", detail.Revision)
	}

	// 未发布：前台不可见
	list, err := svc.CanvasWorkflowTemplates(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if item.Slug == "my-template" {
			t.Fatal("unpublished template leaked into public list")
		}
	}

	// 发布后可见，且版本为 1
	published, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{
		ExpectedRevision: detail.Revision, Name: "首个版本", Notes: "初始",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !published.Published || published.Version != 1 {
		t.Fatalf("publish state = published:%v version:%d", published.Published, published.Version)
	}

	list, err = svc.CanvasWorkflowTemplates(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range list {
		if item.Slug == "my-template" {
			found = true
		}
	}
	if !found {
		t.Fatal("published template missing from public list")
	}
}

func TestWorkflowDraftEditDoesNotAffectPublishedVersion(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)
	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "dual-state", Name: "双态", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: detail.Revision})
	if err != nil {
		t.Fatal(err)
	}

	// 改草稿：只加一个节点
	draft := map[string]any{
		"title": "双态",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "image", "title": "A", "position": map[string]any{"x": 0, "y": 0}, "width": 380, "height": 380, "metadata": map[string]any{"content": "/workflow-samples/sample-1.jpg", "status": "success"}},
			map[string]any{"id": "n2", "type": "image", "title": "B", "position": map[string]any{"x": 400, "y": 0}, "width": 380, "height": 380, "metadata": map[string]any{"content": "/workflow-samples/sample-2.jpg", "status": "success"}},
		},
		"connections": []any{},
	}
	edited, err := svc.SaveCanvasWorkflowDraft(admin, detail.ID, AdminCanvasWorkflowDraftRequest{
		ExpectedRevision: published.Revision, Payload: draft,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !edited.HasUnpublishedDraft {
		t.Fatal("expected unpublished-draft flag")
	}
	if edited.DraftNodeCount != 2 {
		t.Fatalf("draft nodes = %d, want 2", edited.DraftNodeCount)
	}
	if edited.NodeCount != 1 {
		t.Fatalf("published nodes = %d, want 1 (draft must not leak)", edited.NodeCount)
	}

	// 前台仍然只看到 1 个节点的已发布版本
	public, err := svc.CanvasWorkflowTemplate(admin.ID, "dual-state")
	if err != nil {
		t.Fatal(err)
	}
	if public.NodeCount != 1 {
		t.Fatalf("public nodeCount = %d, want 1", public.NodeCount)
	}
}

func TestWorkflowCatalogMetadataIsDraftOnlyUntilPublish(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)
	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "catalog-metadata", Name: "目录资料", Description: "旧卡片简介", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: detail.Revision})
	if err != nil {
		t.Fatal(err)
	}

	description := "新卡片简介"
	guide := "## 使用步骤\n1. 上传商品图片"
	cover := &model.Resource{ID: "cover-resource", UserID: admin.ID, Kind: "image", Status: model.ResourceStatusReady, MimeType: "image/jpeg"}
	if err := svc.repo.CreateResource(cover); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateCanvasWorkflowCoverDraft(&model.CanvasWorkflowCoverDraft{ResourceID: cover.ID, UserID: admin.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateCanvasWorkflowMeta(admin, detail.ID, AdminCanvasWorkflowMetaRequest{
		ExpectedRevision: first.Revision, Description: &description, UsageGuide: &guide, CoverResourceID: &cover.ID,
	}); err != nil {
		t.Fatal(err)
	}
	public, err := svc.CanvasWorkflowTemplate(admin.ID, detail.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if public.Description != "旧卡片简介" || public.UsageGuide != "" || public.CoverURL != "" {
		t.Fatalf("unpublished catalog metadata leaked: description=%q guide=%q cover=%q", public.Description, public.UsageGuide, public.CoverURL)
	}

	adminDetail, err := svc.AdminCanvasWorkflow(admin, detail.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: adminDetail.Revision})
	if err != nil {
		t.Fatal(err)
	}
	public, err = svc.CanvasWorkflowTemplate(admin.ID, detail.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 || public.Description != description || public.UsageGuide != guide || public.CoverURL == "" {
		t.Fatalf("published catalog metadata mismatch: version=%d description=%q guide=%q cover=%q", second.Version, public.Description, public.UsageGuide, public.CoverURL)
	}
	if !containsWorkflowMediaType(public.MediaTypes, "image") {
		t.Fatalf("media types = %#v, want image", public.MediaTypes)
	}
	if _, err := svc.repo.CanvasWorkflowCoverDraftForUser(admin.ID, cover.ID); err == nil {
		t.Fatal("published cover upload draft was not consumed")
	}
	if referenced, err := svc.repo.CanvasWorkflowCoverResourceReferenced(cover.ID); err != nil || !referenced {
		t.Fatalf("published cover resource reference = %v, err=%v; want referenced", referenced, err)
	}
	if err := svc.repo.CreateCanvasWorkflowCoverDraft(&model.CanvasWorkflowCoverDraft{ResourceID: cover.ID, UserID: admin.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DiscardCanvasWorkflowCoverDraft(admin.ID, cover, nil); err == nil {
		t.Fatal("published version cover must not be deleted")
	}
	if _, err := svc.repo.Resource(cover.ID); err != nil {
		t.Fatalf("published cover resource was removed: %v", err)
	}
	thirdDescription := "后续版本简介"
	removeCover := ""
	afterEdit, err := svc.UpdateCanvasWorkflowMeta(admin, detail.ID, AdminCanvasWorkflowMetaRequest{
		ExpectedRevision: second.Revision, Description: &thirdDescription, CoverResourceID: &removeCover,
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: afterEdit.Revision})
	if err != nil {
		t.Fatal(err)
	}
	rolledBack, err := svc.RollbackCanvasWorkflow(admin, detail.ID, second.LatestVersionID, third.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.DraftDescription != description || rolledBack.DraftUsageGuide != guide || rolledBack.DraftCoverResourceID != cover.ID {
		t.Fatalf("rollback did not restore published catalog metadata: %#v", rolledBack)
	}
}

func TestWorkflowRollbackWritesDraftOnly(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)
	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "rollback", Name: "回滚", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	v1, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: detail.Revision})
	if err != nil {
		t.Fatal(err)
	}
	v1ID := v1.LatestVersionID
	if v1ID == "" {
		v1ID = v1.History[0].ID
	}

	// 发布 v2
	draft2 := map[string]any{
		"title": "回滚",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "image", "title": "A", "position": map[string]any{"x": 0, "y": 0}, "width": 380, "height": 380, "metadata": map[string]any{"content": "/workflow-samples/sample-1.jpg", "status": "success"}},
			map[string]any{"id": "n2", "type": "image", "title": "B", "position": map[string]any{"x": 400, "y": 0}, "width": 380, "height": 380, "metadata": map[string]any{"content": "/workflow-samples/sample-2.jpg", "status": "success"}},
		},
		"connections": []any{},
	}
	afterDraft, err := svc.SaveCanvasWorkflowDraft(admin, detail.ID, AdminCanvasWorkflowDraftRequest{ExpectedRevision: v1.Revision, Payload: draft2})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: afterDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if v2.Version != 2 {
		t.Fatalf("version = %d, want 2", v2.Version)
	}

	// 回滚到 v1：只改草稿，线上仍是 v2
	rolled, err := svc.RollbackCanvasWorkflow(admin, detail.ID, v1ID, v2.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Version != 2 {
		t.Fatalf("rollback must not change published version, got %d", rolled.Version)
	}
	if rolled.DraftNodeCount != 1 {
		t.Fatalf("rollback draft nodes = %d, want 1", rolled.DraftNodeCount)
	}
	if rolled.NodeCount != 2 {
		t.Fatalf("published nodes = %d, want 2", rolled.NodeCount)
	}
}

func TestWorkflowDisabledHidesFromPublic(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)
	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "toggle", Name: "启停", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: detail.Revision})
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := svc.UpdateCanvasWorkflowMeta(admin, detail.ID, AdminCanvasWorkflowMetaRequest{
		ExpectedRevision: published.Revision, Enabled: &disabled,
	}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.CanvasWorkflowTemplates(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if item.Slug == "toggle" {
			t.Fatal("disabled template must be hidden from public list")
		}
	}
}

func TestWorkflowRejectsRuntimeFieldsAndPrivateResources(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)

	bad := map[string]any{
		"title": "bad",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "image", "title": "A", "position": map[string]any{"x": 0, "y": 0}, "width": 380, "height": 380, "metadata": map[string]any{"taskId": "leak", "content": "/workflow-samples/sample-1.jpg"}},
		},
		"connections": []any{},
	}
	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "guard", Name: "护栏", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveCanvasWorkflowDraft(admin, detail.ID, AdminCanvasWorkflowDraftRequest{ExpectedRevision: detail.Revision, Payload: bad}); err == nil {
		t.Fatal("expected rejection for runtime field")
	}

	private := map[string]any{
		"title": "bad2",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "image", "title": "A", "position": map[string]any{"x": 0, "y": 0}, "width": 380, "height": 380, "metadata": map[string]any{"content": "/api/resources/abc/file"}},
		},
		"connections": []any{},
	}
	if _, err := svc.SaveCanvasWorkflowDraft(admin, detail.ID, AdminCanvasWorkflowDraftRequest{ExpectedRevision: detail.Revision, Payload: private}); err == nil {
		t.Fatal("expected rejection for private resource reference")
	}
}

func TestWorkflowImportRecordsVersionAndHints(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)
	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "hint", Name: "提示", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: detail.Revision})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := svc.ImportCanvasWorkflowTemplate(admin.ID, "hint")
	if err != nil {
		t.Fatal(err)
	}

	record, err := svc.repo.CanvasWorkflowImportForCanvas(imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != 1 {
		t.Fatalf("import version = %d, want 1", record.Version)
	}

	// 无更新时不应有提示
	hints, err := svc.CanvasWorkflowUpdateHints(admin.ID, []string{imported.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(hints) != 0 {
		t.Fatalf("unexpected hints: %#v", hints)
	}

	// 发布 v2 后应提示
	draft2 := map[string]any{
		"title": "提示",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "image", "title": "A", "position": map[string]any{"x": 0, "y": 0}, "width": 380, "height": 380, "metadata": map[string]any{"content": "/workflow-samples/sample-1.jpg"}},
			map[string]any{"id": "n2", "type": "image", "title": "B", "position": map[string]any{"x": 400, "y": 0}, "width": 380, "height": 380, "metadata": map[string]any{"content": "/workflow-samples/sample-2.jpg"}},
		},
		"connections": []any{},
	}
	edited, err := svc.SaveCanvasWorkflowDraft(admin, detail.ID, AdminCanvasWorkflowDraftRequest{ExpectedRevision: published.Revision, Payload: draft2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: edited.Revision}); err != nil {
		t.Fatal(err)
	}
	hints, err = svc.CanvasWorkflowUpdateHints(admin.ID, []string{imported.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(hints) != 1 || hints[0].ImportedVersion != 1 || hints[0].LatestVersion != 2 {
		t.Fatalf("hints = %#v", hints)
	}
}

func TestWorkflowImportStatsAggregate(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)
	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "stats", Name: "统计", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishCanvasWorkflow(admin, detail.ID, AdminCanvasWorkflowPublishRequest{ExpectedRevision: detail.Revision}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.ImportCanvasWorkflowTemplate(admin.ID, "stats"); err != nil {
			t.Fatal(err)
		}
	}
	list, err := svc.AdminCanvasWorkflows(admin)
	if err != nil {
		t.Fatal(err)
	}
	var target *AdminCanvasWorkflowSummary
	for i := range list {
		if list[i].Slug == "stats" {
			target = &list[i]
		}
	}
	if target == nil {
		t.Fatal("template missing from admin list")
	}
	if target.ImportTotal != 3 {
		t.Fatalf("importTotal = %d, want 3", target.ImportTotal)
	}
	if target.ImportLast7Days != 3 {
		t.Fatalf("importLast7Days = %d, want 3", target.ImportLast7Days)
	}
}

func TestWorkflowDraftRequiresMatchingRevision(t *testing.T) {
	svc, admin, canvas := newWorkflowAdminFixture(t)
	detail, err := svc.CreateCanvasWorkflowFromCanvas(admin, AdminCanvasWorkflowCreateRequest{
		CanvasID: canvas.ID, Slug: "conflict", Name: "冲突", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"title": "x", "nodes": []any{map[string]any{"id": "n1", "type": "text", "title": "t", "position": map[string]any{"x": 0, "y": 0}, "width": 340, "height": 240, "metadata": map[string]any{"content": "hi"}}}, "connections": []any{}}
	if _, err := svc.SaveCanvasWorkflowDraft(admin, detail.ID, AdminCanvasWorkflowDraftRequest{ExpectedRevision: detail.Revision + 99, Payload: payload}); err == nil {
		t.Fatal("expected revision conflict")
	}
}

func TestWorkflowPayloadRejectsOversizedAndTooManyNodes(t *testing.T) {
	nodes := make([]any, 0, 41)
	for i := 0; i < 41; i++ {
		nodes = append(nodes, map[string]any{"id": "n" + string(rune('a'+i%26)), "type": "text", "title": "t", "position": map[string]any{"x": 0, "y": 0}, "width": 340, "height": 240, "metadata": map[string]any{"content": "x"}})
	}
	encoded, _ := json.Marshal(map[string]any{"title": "big", "nodes": nodes, "connections": []any{}})
	if err := validateCanvasWorkflowPayload(string(encoded)); err == nil {
		t.Fatal("expected rejection for >40 nodes")
	}
}

func TestBuiltinTemplateKeepsAdminEdits(t *testing.T) {
	svc, admin, _ := newWorkflowAdminFixture(t)

	// 首次读取会创建内置模板并发布 v1
	if _, err := svc.CanvasWorkflowTemplates(admin.ID); err != nil {
		t.Fatal(err)
	}
	item, err := svc.repo.CanvasWorkflowTemplateBySlug(ecommerceCanvasWorkflowSlug)
	if err != nil {
		t.Fatal(err)
	}

	// 管理员在后台改草稿
	edited := map[string]any{
		"title": "管理员改过的模板",
		"nodes": []any{
			map[string]any{"id": "only-one", "type": "text", "title": "唯一节点", "position": map[string]any{"x": 0, "y": 0}, "width": 340, "height": 240, "metadata": map[string]any{"content": "admin edit"}},
		},
		"connections": []any{},
	}
	if _, err := svc.SaveCanvasWorkflowDraft(admin, item.ID, AdminCanvasWorkflowDraftRequest{ExpectedRevision: item.Revision, Payload: edited}); err != nil {
		t.Fatal(err)
	}

	// 再次读取前台列表：内置模板的代码默认内容不得覆盖管理员草稿
	if _, err := svc.CanvasWorkflowTemplates(admin.ID); err != nil {
		t.Fatal(err)
	}
	after, err := svc.repo.CanvasWorkflowTemplateBySlug(ecommerceCanvasWorkflowSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after.DraftJSON, "admin edit") {
		t.Fatalf("admin draft was overwritten by builtin seed: %s", after.DraftJSON[:200])
	}
	if after.Revision <= item.Revision {
		t.Fatalf("revision did not advance: %d -> %d", item.Revision, after.Revision)
	}
}
