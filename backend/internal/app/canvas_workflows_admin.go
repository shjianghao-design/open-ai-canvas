package app

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/gorm"
)

var canvasWorkflowSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ---- 后台读写模型 ----

// AdminCanvasWorkflowSummary 是后台列表项：比前台多出草稿状态、启停、统计。
type AdminCanvasWorkflowSummary struct {
	ID              string `json:"id"`
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Icon            string `json:"icon"`
	SortOrder       int    `json:"sortOrder"`
	Enabled         bool   `json:"enabled"`
	Published       bool   `json:"published"`
	Builtin         bool   `json:"builtin"`
	Version         int    `json:"version"`
	LatestVersionID string `json:"latestVersionId"`
	Revision        int64  `json:"revision"`
	NodeCount       int    `json:"nodeCount"`
	DraftNodeCount  int    `json:"draftNodeCount"`
	// HasUnpublishedDraft 表示草稿与线上版本内容不同，后台需要提示「有未发布改动」。
	HasUnpublishedDraft bool      `json:"hasUnpublishedDraft"`
	ImportTotal         int64     `json:"importTotal"`
	ImportLast7Days     int64     `json:"importLast7Days"`
	ImportLast30Days    int64     `json:"importLast30Days"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// AdminCanvasWorkflowDetail 是后台详情：草稿 + 线上快照 + 版本历史。
type AdminCanvasWorkflowDetail struct {
	AdminCanvasWorkflowSummary
	DraftDescription     string                       `json:"draftDescription"`
	DraftUsageGuide      string                       `json:"draftUsageGuide"`
	DraftCoverResourceID string                       `json:"draftCoverResourceId,omitempty"`
	DraftCoverURL        string                       `json:"draftCoverUrl,omitempty"`
	Draft                map[string]any               `json:"draft"`
	Latest               map[string]any               `json:"latest,omitempty"`
	History              []AdminCanvasWorkflowVersion `json:"history"`
}

type AdminCanvasWorkflowVersion struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	Notes     string    `json:"notes"`
	NodeCount int       `json:"nodeCount"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	IsLatest  bool      `json:"isLatest"`
}

// AdminCanvasWorkflowDraftRequest 保存草稿。
type AdminCanvasWorkflowDraftRequest struct {
	ExpectedRevision int64          `json:"expectedRevision"`
	Payload          map[string]any `json:"payload"`
}

// AdminCanvasWorkflowMetaRequest 改元数据与启停。
type AdminCanvasWorkflowMetaRequest struct {
	ExpectedRevision int64   `json:"expectedRevision"`
	Name             *string `json:"name,omitempty"`
	Description      *string `json:"description,omitempty"`
	Category         *string `json:"category,omitempty"`
	Icon             *string `json:"icon,omitempty"`
	SortOrder        *int    `json:"sortOrder,omitempty"`
	Enabled          *bool   `json:"enabled,omitempty"`
	UsageGuide       *string `json:"usageGuide,omitempty"`
	CoverResourceID  *string `json:"coverResourceId,omitempty"`
}

// AdminCanvasWorkflowCreateRequest 从画布创建模板（方案 A 的入口）。
type AdminCanvasWorkflowCreateRequest struct {
	CanvasID    string `json:"canvasId"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	SortOrder   int    `json:"sortOrder"`
	Enabled     bool   `json:"enabled"`
}

// AdminCanvasWorkflowPublishRequest 发布草稿。
type AdminCanvasWorkflowPublishRequest struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	Name             string `json:"name"`
	Notes            string `json:"notes"`
}

// ---- 列表 ----

func (s *Service) AdminCanvasWorkflows(actor *model.User) ([]AdminCanvasWorkflowSummary, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	if err := s.ensureBuiltinCanvasWorkflowTemplates(); err != nil {
		return nil, err
	}
	items, err := s.repo.CanvasWorkflowTemplates()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	stats, err := s.repo.CanvasWorkflowImportStats(ids)
	if err != nil {
		// 统计是附加信息，失败不应让后台列表不可用。
		stats = map[string]repository.CanvasWorkflowImportStats{}
	}
	result := make([]AdminCanvasWorkflowSummary, 0, len(items))
	for _, item := range items {
		result = append(result, s.adminCanvasWorkflowSummary(item, stats[item.ID]))
	}
	return result, nil
}

// ---- 详情 ----

func (s *Service) AdminCanvasWorkflow(actor *model.User, id string) (*AdminCanvasWorkflowDetail, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	item, err := s.repo.CanvasWorkflowTemplateByID(id)
	if err != nil {
		return nil, err
	}
	stats, _ := s.repo.CanvasWorkflowImportStats([]string{item.ID})
	versions, err := s.repo.CanvasWorkflowVersions(item.ID)
	if err != nil {
		return nil, err
	}
	detail := &AdminCanvasWorkflowDetail{
		AdminCanvasWorkflowSummary: s.adminCanvasWorkflowSummary(*item, stats[item.ID]),
		DraftDescription:           item.DraftDescription,
		DraftUsageGuide:            item.DraftUsageGuide,
		DraftCoverResourceID:       item.DraftCoverResourceID,
		Draft:                      decodeCanvasWorkflowPayload(item.DraftJSON),
		History:                    make([]AdminCanvasWorkflowVersion, 0, len(versions)),
	}
	if detail.DraftDescription == "" {
		detail.DraftDescription = item.Description
	}
	if item.DraftCoverResourceID != "" {
		detail.DraftCoverURL = "/api/admin/canvas-workflows/" + url.PathEscape(item.ID) + "/draft-cover"
	}
	for _, version := range versions {
		detail.History = append(detail.History, AdminCanvasWorkflowVersion{
			ID: version.ID, Version: version.Version, Name: version.Name, Notes: version.Notes,
			NodeCount: version.NodeCount, CreatedBy: version.CreatedBy, CreatedAt: version.CreatedAt,
			IsLatest: version.ID == item.LatestVersionID,
		})
	}
	if item.LatestVersionID != "" {
		if latest, latestErr := s.repo.CanvasWorkflowVersion(item.LatestVersionID); latestErr == nil {
			detail.Latest = decodeCanvasWorkflowPayload(latest.PayloadJSON)
		}
	}
	return detail, nil
}

// ---- 从画布创建模板（方案 A）----

func (s *Service) CreateCanvasWorkflowFromCanvas(actor *model.User, req AdminCanvasWorkflowCreateRequest) (*AdminCanvasWorkflowDetail, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	slug := strings.ToLower(strings.TrimSpace(req.Slug))
	if !canvasWorkflowSlugPattern.MatchString(slug) {
		return nil, BadAuthRequest("模板标识需为 2-100 位小写字母、数字或连字符")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, BadAuthRequest("请填写模板名称")
	}
	canvas, err := s.repo.CanvasProjectForUser(actor.ID, strings.TrimSpace(req.CanvasID))
	if err != nil {
		return nil, BadAuthRequest("画布不存在或不属于当前账号")
	}
	payload, err := canvasWorkflowPayloadFromCanvas(canvas.PayloadJSON)
	if err != nil {
		return nil, err
	}
	if err := validateCanvasWorkflowPayload(payload); err != nil {
		return nil, err
	}
	now := time.Now()
	item := &model.CanvasWorkflowTemplate{
		ID: kernel.NewID(), Slug: slug, Name: strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description), Category: strings.TrimSpace(req.Category),
		DraftDescription: strings.TrimSpace(req.Description),
		Icon:             strings.TrimSpace(req.Icon), SortOrder: req.SortOrder,
		Enabled: req.Enabled, Published: false, Builtin: false,
		DraftJSON: payload, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.SaveCanvasWorkflowTemplate(item); err != nil {
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "canvas_workflow.create", "canvas_workflow", item.ID,
		"从画布创建模板", map[string]any{"slug": slug, "canvasId": req.CanvasID}); err != nil {
		return nil, err
	}
	return s.AdminCanvasWorkflow(actor, item.ID)
}

// canvasWorkflowPayloadFromCanvas 把画布文档转成模板载荷：
// 剥离运行时字段、重建 ID 无关结构、保留视觉与提示词。
func canvasWorkflowPayloadFromCanvas(raw string) (string, error) {
	var document map[string]any
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return "", BadAuthRequest("画布数据格式无效")
	}
	project := map[string]any{
		"id":             "template",
		"title":          kernel.StringValue(document["title"]),
		"nodes":          document["nodes"],
		"connections":    document["connections"],
		"chatSessions":   []any{},
		"activeChatId":   nil,
		"backgroundMode": document["backgroundMode"],
		"showImageInfo":  document["showImageInfo"],
		"viewport":       document["viewport"],
	}
	rawNodes, _ := project["nodes"].([]any)
	for _, rawNode := range rawNodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		metadata, _ := node["metadata"].(map[string]any)
		if metadata == nil {
			continue
		}
		metadata["status"] = "success"
		for _, key := range []string{"taskId", "generationTaskId", "taskStatus", "taskProgress", "generationBatches", "agentDraftRunId", "errorDetails", "apiKey", "storageKey"} {
			delete(metadata, key)
		}
	}
	encoded, err := json.Marshal(project)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ---- 在线编辑会话 ----

// CanvasWorkflowEditSession 是「在画布中编辑模板草稿」的返回结果。
type CanvasWorkflowEditSession struct {
	CanvasID     string `json:"canvasId"`
	TemplateID   string `json:"templateId"`
	TemplateName string `json:"templateName"`
}

// OpenCanvasWorkflowEditSession 把模板草稿物化成一张临时画布，交给原生画布编辑器打开。
//
// 设计取舍：
//   - 每次打开都新建一张画布，避免与上一次未保存的编辑互相覆盖。
//   - 画布 metadata 记录 workflowTemplateEditId，用于「保存回模板」时校验归属。
//   - 保存回模板成功后由前端调用删除接口清理临时画布；用户中途放弃时它只是一张普通画布，
//     不会污染模板库。
func (s *Service) OpenCanvasWorkflowEditSession(actor *model.User, templateID string) (*CanvasWorkflowEditSession, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	item, err := s.repo.CanvasWorkflowTemplateByID(templateID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(item.DraftJSON) == "" {
		return nil, BadAuthRequest("模板草稿为空，无法编辑")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(item.DraftJSON), &document); err != nil {
		return nil, BadAuthRequest("模板草稿格式无效")
	}
	now := time.Now()
	canvasID := kernel.NewID()
	document["id"] = canvasID
	document["title"] = "[模板编辑] " + item.Name
	document["createdAt"] = now
	document["updatedAt"] = now
	document["chatSessions"] = []any{}
	document["activeChatId"] = nil
	if _, ok := document["viewport"]; !ok {
		document["viewport"] = map[string]any{"x": 0.0, "y": 0.0, "k": 1.0}
	}
	payload, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	canvas := &model.CanvasProject{
		ID: canvasID, UserID: actor.ID, Title: "[模板编辑] " + item.Name,
		PayloadJSON: string(payload), Revision: 0, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.UpsertCanvasProject(canvas); err != nil {
		return nil, err
	}
	return &CanvasWorkflowEditSession{CanvasID: canvasID, TemplateID: item.ID, TemplateName: item.Name}, nil
}

// SaveCanvasWorkflowDraftFromCanvas 把画布当前内容写回模板草稿，并清理临时画布。
// 这是「在画布中编辑 → 保存」的收口点：只改草稿，不发布。
func (s *Service) SaveCanvasWorkflowDraftFromCanvas(actor *model.User, templateID, canvasID string, expectedRevision int64, deleteCanvas bool) (*AdminCanvasWorkflowDetail, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	canvas, err := s.repo.CanvasProjectForUser(actor.ID, strings.TrimSpace(canvasID))
	if err != nil {
		return nil, BadAuthRequest("编辑画布不存在或不属于当前账号")
	}
	payload, err := canvasWorkflowPayloadFromCanvas(canvas.PayloadJSON)
	if err != nil {
		return nil, err
	}
	if err := validateCanvasWorkflowPayload(payload); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateCanvasWorkflowDraft(templateID, expectedRevision, payload, nil); err != nil {
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "canvas_workflow.draft.save_from_canvas", "canvas_workflow", templateID,
		"在画布中编辑模板草稿并保存", map[string]any{"canvasId": canvasID}); err != nil {
		return nil, err
	}
	if deleteCanvas {
		// 清理失败不影响草稿已保存的事实。
		_ = s.repo.DeleteCanvasProject(actor.ID, canvasID)
	}
	return s.AdminCanvasWorkflow(actor, templateID)
}

// ---- 保存草稿 ----

func (s *Service) SaveCanvasWorkflowDraft(actor *model.User, id string, req AdminCanvasWorkflowDraftRequest) (*AdminCanvasWorkflowDetail, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(req.Payload)
	if err != nil {
		return nil, BadAuthRequest("草稿数据格式无效")
	}
	if err := validateCanvasWorkflowPayload(string(encoded)); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateCanvasWorkflowDraft(id, req.ExpectedRevision, string(encoded), nil); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, BadAuthRequest("模板不存在")
		}
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "canvas_workflow.draft.save", "canvas_workflow", id,
		"保存模板草稿", map[string]any{"revision": req.ExpectedRevision}); err != nil {
		return nil, err
	}
	return s.AdminCanvasWorkflow(actor, id)
}

// ---- 元数据与启停 ----

func (s *Service) UpdateCanvasWorkflowMeta(actor *model.User, id string, req AdminCanvasWorkflowMetaRequest) (*AdminCanvasWorkflowDetail, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	item, err := s.repo.CanvasWorkflowTemplateByID(id)
	if err != nil {
		return nil, err
	}
	patch := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, BadAuthRequest("模板名称不能为空")
		}
		patch["name"] = name
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if utf8.RuneCountInString(description) > 500 {
			return nil, BadAuthRequest("卡片简介不能超过 500 个字符")
		}
		patch["draft_description"] = description
	}
	if req.UsageGuide != nil {
		guide := strings.TrimSpace(*req.UsageGuide)
		if len(guide) > 20000 {
			return nil, BadAuthRequest("使用指南不能超过 20000 字节")
		}
		patch["draft_usage_guide"] = guide
	}
	if req.CoverResourceID != nil {
		coverID := strings.TrimSpace(*req.CoverResourceID)
		if coverID != "" && coverID != item.DraftCoverResourceID {
			if _, err := s.validateCanvasWorkflowCoverDraft(actor, coverID); err != nil {
				return nil, err
			}
		}
		patch["draft_cover_resource_id"] = coverID
	}
	if req.Category != nil {
		patch["category"] = strings.TrimSpace(*req.Category)
	}
	if req.Icon != nil {
		patch["icon"] = strings.TrimSpace(*req.Icon)
	}
	if req.SortOrder != nil {
		patch["sort_order"] = *req.SortOrder
	}
	if req.Enabled != nil {
		patch["enabled"] = *req.Enabled
	}
	if len(patch) == 0 {
		return nil, BadAuthRequest("没有需要修改的字段")
	}
	if err := s.repo.UpdateCanvasWorkflowMeta(id, req.ExpectedRevision, patch); err != nil {
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "canvas_workflow.update", "canvas_workflow", id,
		"更新模板元数据或启停状态", patch); err != nil {
		return nil, err
	}
	return s.AdminCanvasWorkflow(actor, id)
}

// ---- 发布 ----

func (s *Service) PublishCanvasWorkflow(actor *model.User, id string, req AdminCanvasWorkflowPublishRequest) (*AdminCanvasWorkflowDetail, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	item, err := s.repo.CanvasWorkflowTemplateByID(id)
	if err != nil {
		return nil, err
	}
	if item.Revision != req.ExpectedRevision {
		return nil, repository.ErrCanvasRevisionConflict
	}
	version, err := s.publishCanvasWorkflow(id, req.Name, req.Notes)
	if err != nil {
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "canvas_workflow.publish", "canvas_workflow", id,
		"发布模板新版本", map[string]any{"version": version.Version, "versionId": version.ID}); err != nil {
		return nil, err
	}
	return s.AdminCanvasWorkflow(actor, id)
}

// ---- 回滚 ----

// RollbackCanvasWorkflow 把历史版本内容写回草稿，不直接上线。
// 与 Dify 一致：回滚只改草稿，管理员确认后再发布，避免误操作影响线上。
func (s *Service) RollbackCanvasWorkflow(actor *model.User, id, versionID string, expectedRevision int64) (*AdminCanvasWorkflowDetail, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	version, err := s.repo.CanvasWorkflowVersion(versionID)
	if err != nil {
		return nil, BadAuthRequest("版本不存在")
	}
	if version.TemplateID != id {
		return nil, BadAuthRequest("版本不属于该模板")
	}
	if err := s.repo.UpdateCanvasWorkflowDraft(id, expectedRevision, version.PayloadJSON, map[string]any{
		"draft_description":       version.Description,
		"draft_usage_guide":       version.UsageGuide,
		"draft_cover_resource_id": version.CoverResourceID,
	}); err != nil {
		return nil, err
	}
	if err := s.appendAdminAudit(actor, "canvas_workflow.rollback", "canvas_workflow", id,
		"回滚模板草稿到历史版本", map[string]any{"version": version.Version, "versionId": version.ID}); err != nil {
		return nil, err
	}
	return s.AdminCanvasWorkflow(actor, id)
}

// ---- 删除（归档）----

func (s *Service) DeleteCanvasWorkflow(actor *model.User, id string) error {
	if err := s.RequireAdmin(actor); err != nil {
		return err
	}
	item, err := s.repo.CanvasWorkflowTemplateByID(id)
	if err != nil {
		return err
	}
	if item.Builtin {
		return BadAuthRequest("内置模板不能删除，请改为停用")
	}
	if err := s.repo.DeleteCanvasWorkflowTemplate(id); err != nil {
		return err
	}
	return s.appendAdminAudit(actor, "canvas_workflow.delete", "canvas_workflow", id,
		"删除模板", map[string]any{"slug": item.Slug})
}

// ---- 版本更新提示 ----

// CanvasWorkflowUpdateHint 告诉用户：你导入的画布来自旧版本，模板已更新。
type CanvasWorkflowUpdateHint struct {
	TemplateSlug    string `json:"templateSlug"`
	TemplateName    string `json:"templateName"`
	ImportedVersion int    `json:"importedVersion"`
	LatestVersion   int    `json:"latestVersion"`
}

// CanvasWorkflowUpdateHints 返回该用户所有「导入过且模板已更新」的画布提示。
// 只提示，不强制；用户可继续使用自己的副本。
func (s *Service) CanvasWorkflowUpdateHints(userID string, canvasIDs []string) ([]CanvasWorkflowUpdateHint, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, BadAuthRequest("请先登录")
	}
	hints := make([]CanvasWorkflowUpdateHint, 0)
	for _, canvasID := range canvasIDs {
		record, err := s.repo.CanvasWorkflowImportForCanvas(canvasID)
		if err != nil || record.UserID != userID {
			continue
		}
		template, err := s.repo.CanvasWorkflowTemplateByID(record.TemplateID)
		if err != nil || !template.Enabled || template.LatestVersionID == "" {
			continue
		}
		if record.VersionID == template.LatestVersionID {
			continue
		}
		hints = append(hints, CanvasWorkflowUpdateHint{
			TemplateSlug: template.Slug, TemplateName: template.Name,
			ImportedVersion: record.Version, LatestVersion: template.Version,
		})
	}
	return hints, nil
}

// ---- helpers ----

func (s *Service) adminCanvasWorkflowSummary(item model.CanvasWorkflowTemplate, stats repository.CanvasWorkflowImportStats) AdminCanvasWorkflowSummary {
	summary := AdminCanvasWorkflowSummary{
		ID: item.ID, Slug: item.Slug, Name: item.Name, Description: item.Description,
		Category: item.Category, Icon: item.Icon, SortOrder: item.SortOrder,
		Enabled: item.Enabled, Published: item.Published, Builtin: item.Builtin,
		Version: item.Version, LatestVersionID: item.LatestVersionID, Revision: item.Revision,
		DraftNodeCount: canvasWorkflowNodeCount(item.DraftJSON),
		ImportTotal:    stats.Total, ImportLast7Days: stats.Last7Days, ImportLast30Days: stats.Last30Days,
		UpdatedAt: item.UpdatedAt,
	}
	// 线上节点数取最新快照，草稿节点数单独展示，两者不同即代表「有未发布改动」。
	if item.LatestVersionID != "" {
		if latest, err := s.repo.CanvasWorkflowVersion(item.LatestVersionID); err == nil {
			summary.NodeCount = latest.NodeCount
			summary.HasUnpublishedDraft = latest.PayloadJSON != item.DraftJSON
		}
	}
	if summary.DraftNodeCount > 0 && summary.NodeCount == 0 {
		summary.HasUnpublishedDraft = true
	}
	return summary
}

func decodeCanvasWorkflowPayload(payload string) map[string]any {
	if strings.TrimSpace(payload) == "" {
		return map[string]any{}
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		return map[string]any{}
	}
	return document
}
