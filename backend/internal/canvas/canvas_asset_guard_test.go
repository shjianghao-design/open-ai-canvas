package canvas

import (
	"encoding/json"
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

const deletedResourceID = "resource-deleted-hard"

// 复现线上画布：素材被硬删除（resources 没有软删除列）后，画布节点仍留着它的引用。
func seedCanvasWithDeletedResource(t *testing.T, svc *Service) {
	t.Helper()
	if err := svc.repo.Create(&model.Resource{
		ID: deletedResourceID, UserID: "user", Status: model.ResourceStatusReady,
		Provider: "local", ObjectKey: "users/user/image/lost.png",
	}); err != nil {
		t.Fatal(err)
	}
	payload := `{"id":"canvas","title":"自由画布 1","nodes":[{"id":"image-node","type":"image","metadata":{` +
		`"assetId":"asset-lost","storageKey":"resource:` + deletedResourceID + `","content":"/api/resources/` + deletedResourceID + `/file"}}],"connections":[]}`
	if err := svc.repo.Create(&model.CanvasProject{ID: "canvas", UserID: "user", Title: "自由画布 1", PayloadJSON: payload}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.Delete(&model.Resource{}, "id = ?", deletedResourceID); err != nil {
		t.Fatal(err)
	}
}

// 素材被删掉后仍要能继续保存画布：客户端没有入口移除节点上的失效引用，严格拒绝只会让
// 整张画布在之后每一次编辑（新建节点、连线、改标题）都 409，既修不好数据也拦不住新损坏。
func TestApplyCanvasOperationsAllowsPreexistingMissingResourceReference(t *testing.T) {
	svc := newCanvasOperationTestService(t)
	seedCanvasWithDeletedResource(t, svc)

	result, err := svc.ApplyUserCanvasOperations("user", "canvas", CanvasOperationsRequest{
		MutationID: "mutation-after-resource-deleted",
		Operations: []CanvasOperation{{
			Type: "create_node",
			Node: json.RawMessage(`{"id":"script-node","type":"script","title":"分镜脚本","metadata":{"content":"脚本正文"}}`),
		}},
	})
	if err != nil {
		t.Fatalf("素材被删除后画布仍应可保存，实际 error = %v", err)
	}
	if result.Revision <= 0 {
		t.Fatalf("revision = %d", result.Revision)
	}
	document, err := svc.UserCanvasProject("user", "canvas")
	if err != nil {
		t.Fatal(err)
	}
	// 不擅自改写用户数据：失效引用原样保留，只是不再阻断保存。
	if !strings.Contains(string(document), deletedResourceID) || !strings.Contains(string(document), "script-node") {
		t.Fatalf("document = %s", document)
	}
}

// 反向保护：本次变更新引入的缺失引用仍然必须被拒绝，否则守卫就形同废弃。
func TestApplyCanvasOperationsRejectsNewlyIntroducedMissingResourceReference(t *testing.T) {
	svc := newCanvasOperationTestService(t)
	if err := svc.repo.Create(&model.CanvasProject{
		ID: "canvas", UserID: "user", Title: "画布",
		PayloadJSON: `{"id":"canvas","title":"画布","nodes":[{"id":"text-node","type":"text","metadata":{"content":"备注"}}],"connections":[]}`,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.ApplyUserCanvasOperations("user", "canvas", CanvasOperationsRequest{
		MutationID: "mutation-new-broken-media",
		Operations: []CanvasOperation{{
			Type: "create_node",
			Node: json.RawMessage(`{"id":"image-node","type":"image","metadata":{` +
				`"assetId":"asset-new-missing","storageKey":"resource:` + deletedResourceID + `","content":"/api/resources/` + deletedResourceID + `/file"}}`),
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "画布引用的素材已变化") {
		t.Fatalf("新引入的失效引用必须继续拒绝，实际 error = %v", err)
	}
}

// 整篇文档覆盖保存走另一条提交路径（历史快照的资源核对），同一份数据不能在那一步再被拒。
func TestUpsertUserCanvasProjectAllowsPreexistingMissingResourceReference(t *testing.T) {
	svc := newCanvasHistoryTestService(t)
	if err := svc.repo.Create(&model.Resource{
		ID: deletedResourceID, UserID: "user", Status: model.ResourceStatusReady,
		Provider: "local", ObjectKey: "users/user/image/lost.png",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.Create(&model.Asset{
		ID: "asset-node", UserID: "user",
		PayloadJSON: `{"id":"asset-node","data":{"storageKey":"resource:` + deletedResourceID + `"}}`,
	}); err != nil {
		t.Fatal(err)
	}
	node := `{"id":"image-node","type":"image","metadata":{"assetId":"asset-node","storageKey":"resource:` + deletedResourceID + `"}}`
	if _, err := svc.UpsertUserCanvasProject("user", json.RawMessage(
		`{"id":"canvas","revision":0,"title":"画布","nodes":[`+node+`],"connections":[]}`)); err != nil {
		t.Fatalf("首次保存 error = %v", err)
	}
	if err := svc.repo.Delete(&model.Resource{}, "id = ?", deletedResourceID); err != nil {
		t.Fatal(err)
	}

	result, err := svc.UpsertUserCanvasProject("user", json.RawMessage(
		`{"id":"canvas","revision":1,"title":"画布","nodes":[`+node+`,{"id":"text-node","type":"text","metadata":{"content":"备注"}}],"connections":[]}`))
	if err != nil {
		t.Fatalf("素材删除后覆盖保存 error = %v", err)
	}
	if result.Revision != 2 {
		t.Fatalf("revision = %d, want 2", result.Revision)
	}
	list, err := svc.CanvasHistory("user", "canvas")
	if err != nil || len(list.Snapshots) != 1 {
		t.Fatalf("历史快照应正常归档: %#v %v", list, err)
	}
	resources, err := svc.repo.CanvasHistoryResourceReferences([]string{deletedResourceID})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Fatalf("失效资源不应写入快照引用表: %#v", resources)
	}
}
