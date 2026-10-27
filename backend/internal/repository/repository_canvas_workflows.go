package repository

import (
	"errors"
	"strings"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

var ErrCanvasWorkflowCoverReferenced = errors.New("workflow cover is referenced by a template or published version")

// ---- 前台读取：只暴露 enabled 且已发布的模板 ----

// VisibleCanvasWorkflowTemplates 返回前台可见模板，按运营排序。
func (r *Repository) VisibleCanvasWorkflowTemplates() ([]model.CanvasWorkflowTemplate, error) {
	var templates []model.CanvasWorkflowTemplate
	err := r.db.
		Where("enabled = ? AND published = ? AND latest_version_id <> ''", true, true).
		Order("sort_order ASC, category ASC, name ASC").
		Find(&templates).Error
	return templates, err
}

// VisibleCanvasWorkflowTemplateBySlug 按 slug 取前台可见模板。
func (r *Repository) VisibleCanvasWorkflowTemplateBySlug(slug string) (*model.CanvasWorkflowTemplate, error) {
	var template model.CanvasWorkflowTemplate
	err := r.db.
		Where("slug = ? AND enabled = ? AND published = ? AND latest_version_id <> ''", strings.TrimSpace(slug), true, true).
		First(&template).Error
	if err != nil {
		return nil, err
	}
	return &template, nil
}

// ---- 后台读取：包含禁用与未发布 ----

func (r *Repository) CanvasWorkflowTemplates() ([]model.CanvasWorkflowTemplate, error) {
	var templates []model.CanvasWorkflowTemplate
	err := r.db.Order("sort_order ASC, category ASC, name ASC").Find(&templates).Error
	return templates, err
}

func (r *Repository) CanvasWorkflowTemplateBySlug(slug string) (*model.CanvasWorkflowTemplate, error) {
	var template model.CanvasWorkflowTemplate
	if err := r.db.Where("slug = ?", strings.TrimSpace(slug)).First(&template).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

func (r *Repository) CanvasWorkflowTemplateByID(id string) (*model.CanvasWorkflowTemplate, error) {
	var template model.CanvasWorkflowTemplate
	if err := r.db.Where("id = ?", strings.TrimSpace(id)).First(&template).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

// ---- 写入 ----

func (r *Repository) SaveCanvasWorkflowTemplate(template *model.CanvasWorkflowTemplate) error {
	return r.db.Save(template).Error
}

// UpdateCanvasWorkflowDraft 用乐观锁写草稿：revision 不匹配就拒绝，避免后台并发覆盖。
func (r *Repository) UpdateCanvasWorkflowDraft(id string, expectedRevision int64, draft string, patch map[string]any) error {
	updates := map[string]any{"draft_json": draft, "revision": gorm.Expr("revision + 1")}
	for key, value := range patch {
		updates[key] = value
	}
	result := r.db.Model(&model.CanvasWorkflowTemplate{}).
		Where("id = ? AND revision = ?", id, expectedRevision).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCanvasRevisionConflict
	}
	return nil
}

// UpdateCanvasWorkflowMeta 只改元数据（启用、发布、排序、名称），同样带乐观锁。
func (r *Repository) UpdateCanvasWorkflowMeta(id string, expectedRevision int64, patch map[string]any) error {
	updates := map[string]any{"revision": gorm.Expr("revision + 1")}
	for key, value := range patch {
		updates[key] = value
	}
	result := r.db.Model(&model.CanvasWorkflowTemplate{}).
		Where("id = ? AND revision = ?", id, expectedRevision).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCanvasRevisionConflict
	}
	return nil
}

// UpdateCanvasWorkflowPublishState 由发布流程调用，把前台指针指向新版本。
func (r *Repository) UpdateCanvasWorkflowPublishState(id, versionID string, version int) error {
	return r.db.Model(&model.CanvasWorkflowTemplate{}).Where("id = ?", id).Updates(map[string]any{
		"latest_version_id": versionID,
		"version":           version,
		"published":         true,
		"revision":          gorm.Expr("revision + 1"),
		"updated_at":        gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error
}

func (r *Repository) SaveCanvasWorkflowCatalogDraft(id string, expectedRevision int64, patch map[string]any) error {
	updates := map[string]any{"revision": gorm.Expr("revision + 1"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
	for key, value := range patch {
		updates[key] = value
	}
	result := r.db.Model(&model.CanvasWorkflowTemplate{}).Where("id = ? AND revision = ?", id, expectedRevision).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCanvasRevisionConflict
	}
	return nil
}

// PublishCanvasWorkflowVersion commits the immutable snapshot and public pointer together.
func (r *Repository) PublishCanvasWorkflowVersion(version *model.CanvasWorkflowVersion, expectedRevision int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		result := tx.Model(&model.CanvasWorkflowTemplate{}).Where("id = ? AND revision = ?", version.TemplateID, expectedRevision).Updates(map[string]any{
			"description": version.Description, "latest_version_id": version.ID, "version": version.Version,
			"published": true, "revision": gorm.Expr("revision + 1"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCanvasRevisionConflict
		}
		if version.CoverResourceID != "" {
			if err := tx.Where("resource_id = ?", version.CoverResourceID).Delete(&model.CanvasWorkflowCoverDraft{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) CreateCanvasWorkflowCoverDraft(draft *model.CanvasWorkflowCoverDraft) error {
	return r.db.Create(draft).Error
}

func (r *Repository) CanvasWorkflowCoverDraftForUser(userID, resourceID string) (*model.CanvasWorkflowCoverDraft, error) {
	var draft model.CanvasWorkflowCoverDraft
	if err := r.db.First(&draft, "resource_id = ? AND user_id = ?", resourceID, userID).Error; err != nil {
		return nil, err
	}
	return &draft, nil
}

func (r *Repository) DeleteCanvasWorkflowCoverDraft(userID, resourceID string) error {
	return r.db.Where("resource_id = ? AND user_id = ?", resourceID, userID).Delete(&model.CanvasWorkflowCoverDraft{}).Error
}

func (r *Repository) StaleCanvasWorkflowCoverDrafts(before time.Time, limit int) ([]model.CanvasWorkflowCoverDraft, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var drafts []model.CanvasWorkflowCoverDraft
	err := r.db.Where("created_at < ?", before).Order("created_at asc").Limit(limit).Find(&drafts).Error
	return drafts, err
}

func (r *Repository) CanvasWorkflowCoverResourceReferenced(resourceID string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.CanvasWorkflowTemplate{}).Where("draft_cover_resource_id = ?", resourceID).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := r.db.Model(&model.CanvasWorkflowVersion{}).Where("cover_resource_id = ?", resourceID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) DiscardCanvasWorkflowCoverDraft(userID string, resource *model.Resource, deletionJob *model.ResourceDeletionJob) error {
	if resource == nil {
		return gorm.ErrRecordNotFound
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var draft model.CanvasWorkflowCoverDraft
		if err := tx.First(&draft, "resource_id = ? AND user_id = ?", resource.ID, userID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.CanvasWorkflowTemplate{}).Where("draft_cover_resource_id = ?", resource.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCanvasWorkflowCoverReferenced
		}
		if err := tx.Model(&model.CanvasWorkflowVersion{}).Where("cover_resource_id = ?", resource.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCanvasWorkflowCoverReferenced
		}
		deleted := tx.Where("id = ? AND user_id = ?", resource.ID, userID).Delete(&model.Resource{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Delete(&draft).Error; err != nil {
			return err
		}
		if deletionJob != nil {
			return tx.Create(deletionJob).Error
		}
		return nil
	})
}

func (r *Repository) DeleteCanvasWorkflowTemplate(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("template_id = ?", id).Delete(&model.CanvasWorkflowVersion{}).Error; err != nil {
			return err
		}
		if err := tx.Where("template_id = ?", id).Delete(&model.CanvasWorkflowImport{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.CanvasWorkflowTemplate{}).Error
	})
}

// ---- 版本 ----

func (r *Repository) CreateCanvasWorkflowVersion(version *model.CanvasWorkflowVersion) error {
	return r.db.Create(version).Error
}

func (r *Repository) CanvasWorkflowVersion(id string) (*model.CanvasWorkflowVersion, error) {
	var version model.CanvasWorkflowVersion
	if err := r.db.Where("id = ?", strings.TrimSpace(id)).First(&version).Error; err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *Repository) CanvasWorkflowVersions(templateID string) ([]model.CanvasWorkflowVersion, error) {
	var versions []model.CanvasWorkflowVersion
	err := r.db.Where("template_id = ?", templateID).Order("version DESC").Find(&versions).Error
	return versions, err
}

// ---- 导入统计 ----

func (r *Repository) SaveCanvasWorkflowImport(record *model.CanvasWorkflowImport) error {
	return r.db.Create(record).Error
}

// CanvasWorkflowImportStats 是一次聚合查询的结果，供后台列表展示。
type CanvasWorkflowImportStats struct {
	TemplateID string
	Total      int64
	Last7Days  int64
	Last30Days int64
}

func (r *Repository) CanvasWorkflowImportStats(templateIDs []string) (map[string]CanvasWorkflowImportStats, error) {
	result := make(map[string]CanvasWorkflowImportStats, len(templateIDs))
	if len(templateIDs) == 0 {
		return result, nil
	}
	// 用 Go 侧时间窗口而不是 SQL 的 INTERVAL 语法：SQLite 与 PostgreSQL 都要能跑。
	now := time.Now()
	cut7 := now.AddDate(0, 0, -7)
	cut30 := now.AddDate(0, 0, -30)

	type row struct {
		TemplateID string
		CreatedAt  time.Time
	}
	var rows []row
	if err := r.db.Model(&model.CanvasWorkflowImport{}).
		Select("template_id, created_at").
		Where("template_id IN ?", templateIDs).
		Scan(&rows).Error; err != nil {
		return result, err
	}
	for _, item := range rows {
		stats := result[item.TemplateID]
		stats.TemplateID = item.TemplateID
		stats.Total++
		if item.CreatedAt.After(cut7) {
			stats.Last7Days++
		}
		if item.CreatedAt.After(cut30) {
			stats.Last30Days++
		}
		result[item.TemplateID] = stats
	}
	return result, nil
}

// CanvasWorkflowImportForCanvas 用于「模板已更新」提示：读取用户画布的导入来源版本。
func (r *Repository) CanvasWorkflowImportForCanvas(canvasID string) (*model.CanvasWorkflowImport, error) {
	var record model.CanvasWorkflowImport
	if err := r.db.Where("canvas_id = ?", canvasID).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}
