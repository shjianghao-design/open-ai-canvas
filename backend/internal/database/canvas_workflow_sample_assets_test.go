package database

import (
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

const samplePNGPayload = `{"nodes":[{"id":"n1","type":"image","metadata":{"content":"/workflow-samples-reference/taobao-main-image/-06241358-2.png","mimeType":"image/png"}},` +
	`{"id":"n2","type":"video","metadata":{"content":"/workflow-samples-reference/outfit-change-video/-09121704.mp4","mimeType":"video/mp4"}},` +
	`{"id":"n3","type":"image","metadata":{"content":"/workflow-samples-reference-211476/front.jpg","mimeType":"image/jpeg"}}]}`

// 编辑器落库的带空格格式，MIME 也要一起改。
const samplePNGPayloadSpaced = `{
  "nodes": [
    {
      "id": "n1",
      "metadata": {
        "content": "/workflow-samples-reference/face-replica/-09101459.png",
        "mimeType": "image/png"
      }
    }
  ]
}`

func TestRewriteCanvasWorkflowSampleAssetsOnlyTouchesSamplePNG(t *testing.T) {
	got := rewriteCanvasWorkflowSampleAssets(samplePNGPayload)

	for _, want := range []string{
		"/workflow-samples-reference/taobao-main-image/-06241358-2.webp",
		`"mimeType":"image/webp"`,
		// 视频与 211476 素材包不受影响
		"/workflow-samples-reference/outfit-change-video/-09121704.mp4",
		`"mimeType":"video/mp4"`,
		"/workflow-samples-reference-211476/front.jpg",
		`"mimeType":"image/jpeg"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("改写后缺少 %q，实际: %s", want, got)
		}
	}
	if strings.Contains(got, "-06241358-2.png") {
		t.Errorf("PNG 路径未被改写: %s", got)
	}
	if strings.Contains(got, `"image/png"`) {
		t.Errorf("PNG MIME 未被改写: %s", got)
	}
	// 幂等：再跑一次不应继续变化
	if again := rewriteCanvasWorkflowSampleAssets(got); again != got {
		t.Errorf("改写不幂等：\n第一次 %s\n第二次 %s", got, again)
	}
}

func TestRewriteCanvasWorkflowSampleAssetsHandlesSpacedJSON(t *testing.T) {
	got := rewriteCanvasWorkflowSampleAssets(samplePNGPayloadSpaced)
	if !strings.Contains(got, "-09101459.webp") {
		t.Errorf("带空格的载荷未改写路径: %s", got)
	}
	if !strings.Contains(got, `"mimeType": "image/webp"`) {
		t.Errorf("带空格的载荷未改写 MIME: %s", got)
	}
}

func TestRewriteCanvasWorkflowSampleAssetsLeavesUnrelatedPayloadUntouched(t *testing.T) {
	raw := `{"nodes":[{"id":"n1","metadata":{"content":"/api/resources/abc/download","mimeType":"image/png"}}]}`
	if got := rewriteCanvasWorkflowSampleAssets(raw); got != raw {
		t.Errorf("不含示例素材的载荷被改动: %s", got)
	}
}

func TestMigrateCanvasWorkflowSampleAssetsToWebP(t *testing.T) {
	db, err := Open(Config{Driver: "sqlite", DSN: "file:workflow-sample-assets?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.CanvasWorkflowTemplate{}, &model.CanvasWorkflowVersion{}); err != nil {
		t.Fatal(err)
	}

	// 模拟存量库：revision 已 >1，代码里的新内容不会覆盖它。
	if err := db.Create(&model.CanvasWorkflowTemplate{
		ID: "t1", Slug: "demo", Name: "Demo", Category: "测试", Revision: 2, Version: 2,
		DraftJSON: samplePNGPayload,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CanvasWorkflowVersion{
		ID: "v1", TemplateID: "t1", Version: 1, PayloadJSON: samplePNGPayload,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateCanvasWorkflowSampleAssetsToWebP(db); err != nil {
		t.Fatal(err)
	}

	var template model.CanvasWorkflowTemplate
	if err := db.First(&template, "id = ?", "t1").Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(template.DraftJSON, "-06241358-2.png") {
		t.Errorf("草稿仍指向 PNG: %s", template.DraftJSON)
	}
	if !strings.Contains(template.DraftJSON, "-06241358-2.webp") {
		t.Errorf("草稿未切换到 WebP: %s", template.DraftJSON)
	}

	var version model.CanvasWorkflowVersion
	if err := db.First(&version, "id = ?", "v1").Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(version.PayloadJSON, "-06241358-2.webp") {
		t.Errorf("版本快照未切换到 WebP: %s", version.PayloadJSON)
	}

	// 重复执行必须是无副作用的空操作。
	if err := migrateCanvasWorkflowSampleAssetsToWebP(db); err != nil {
		t.Fatal(err)
	}
	var again model.CanvasWorkflowTemplate
	if err := db.First(&again, "id = ?", "t1").Error; err != nil {
		t.Fatal(err)
	}
	if again.DraftJSON != template.DraftJSON {
		t.Error("迁移不幂等：重复执行改动了草稿")
	}
}
