package app

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"yingce/backend/internal/assets"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/gorm"
)

const CanvasWorkflowCoverMaxBytes int64 = 10 << 20
const canvasWorkflowCoverDraftTTL = 24 * time.Hour

func (s *Service) UploadCanvasWorkflowCover(actor *model.User, header *multipart.FileHeader) (*model.Resource, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	if err := validateCanvasWorkflowCoverUpload(header); err != nil {
		return nil, err
	}
	resource, err := s.UploadResource(actor.ID, header, "image", 1200, 675, 0)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateCanvasWorkflowCoverDraft(&model.CanvasWorkflowCoverDraft{ResourceID: resource.ID, UserID: actor.ID, CreatedAt: time.Now()}); err != nil {
		if cleanupErr := s.deleteFreshAnnouncementImageResource(resource); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("清理未登记的工作流封面失败：%w", cleanupErr))
		}
		return nil, err
	}
	resource.PublicURL = ""
	return resource, nil
}

func (s *Service) validateCanvasWorkflowCoverDraft(actor *model.User, resourceID string) (string, error) {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" || len(resourceID) > 64 {
		return "", BadAuthRequest("工作流封面资源 ID 无效")
	}
	if _, err := s.repo.CanvasWorkflowCoverDraftForUser(actor.ID, resourceID); err != nil {
		return "", BadAuthRequest("工作流封面上传草稿不存在或不属于当前管理员")
	}
	resource, err := s.repo.ResourceForUser(actor.ID, resourceID)
	if err != nil || resource.Kind != "image" || resource.Status != model.ResourceStatusReady || !strings.HasPrefix(strings.ToLower(resource.MimeType), "image/") {
		return "", BadAuthRequest("工作流封面资源不可用")
	}
	return resourceID, nil
}

func (s *Service) DiscardCanvasWorkflowCover(actor *model.User, resourceID string) error {
	if err := s.RequireAdmin(actor); err != nil {
		return err
	}
	return s.discardCanvasWorkflowCoverDraft(actor.ID, resourceID)
}

func (s *Service) PrepareCanvasWorkflowCoverDelivery(actor *model.User, slug string, options ResourceAccessOptions, rangeHeader string) (*ResourceDelivery, error) {
	if actor == nil {
		return nil, Unauthorized("请先登录")
	}
	template, err := s.repo.VisibleCanvasWorkflowTemplateBySlug(slug)
	if err != nil {
		return nil, err
	}
	if template.LatestVersionID == "" {
		return nil, NotFound("工作流封面不存在")
	}
	version, err := s.repo.CanvasWorkflowVersion(template.LatestVersionID)
	if err != nil {
		return nil, err
	}
	return s.prepareCanvasWorkflowCoverResource(version.CoverResourceID, options, rangeHeader)
}

func (s *Service) PrepareAdminCanvasWorkflowDraftCover(actor *model.User, templateID string, options ResourceAccessOptions, rangeHeader string) (*ResourceDelivery, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	template, err := s.repo.CanvasWorkflowTemplateByID(templateID)
	if err != nil {
		return nil, err
	}
	return s.prepareCanvasWorkflowCoverResource(template.DraftCoverResourceID, options, rangeHeader)
}

func (s *Service) prepareCanvasWorkflowCoverResource(resourceID string, options ResourceAccessOptions, rangeHeader string) (*ResourceDelivery, error) {
	if strings.TrimSpace(resourceID) == "" {
		return nil, NotFound("工作流封面不存在")
	}
	resource, err := s.repo.Resource(resourceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, NotFound("工作流封面不存在")
		}
		return nil, err
	}
	if resource.Kind != "image" || resource.Status != model.ResourceStatusReady {
		return nil, BadAuthRequest("工作流封面资源不可用")
	}
	if options.Purpose == "" {
		options.Purpose = assets.PurposeDisplay
	}
	return s.prepareResourceDelivery(resource.UserID, resource, options, rangeHeader)
}

func (s *Service) cleanupStaleCanvasWorkflowCoverDrafts() {
	drafts, err := s.repo.StaleCanvasWorkflowCoverDrafts(time.Now().Add(-canvasWorkflowCoverDraftTTL), 100)
	if err != nil {
		return
	}
	for _, draft := range drafts {
		referenced, err := s.repo.CanvasWorkflowCoverResourceReferenced(draft.ResourceID)
		if err != nil {
			continue
		}
		if referenced {
			_ = s.repo.DeleteCanvasWorkflowCoverDraft(draft.UserID, draft.ResourceID)
			continue
		}
		_ = s.discardCanvasWorkflowCoverDraft(draft.UserID, draft.ResourceID)
	}
}

func (s *Service) discardCanvasWorkflowCoverDraft(userID, resourceID string) error {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" || len(resourceID) > 64 {
		return BadAuthRequest("工作流封面资源 ID 无效")
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	if _, err := s.repo.CanvasWorkflowCoverDraftForUser(userID, resourceID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return NotFound("工作流封面上传草稿不存在")
		}
		return err
	}
	referenced, err := s.repo.CanvasWorkflowCoverResourceReferenced(resourceID)
	if err != nil {
		return err
	}
	if referenced {
		return BadAuthRequest("工作流封面已被草稿或发布版本引用，不能删除")
	}
	resource, err := s.repo.ResourceForUser(userID, resourceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.repo.DeleteCanvasWorkflowCoverDraft(userID, resourceID)
		}
		return err
	}
	if err := s.ensureResourceHasNoBusinessReferences(resource, repository.ResourceDirectReference{Kind: "工作流封面上传草稿", ID: resourceID}); err != nil {
		return err
	}
	sharedCount, err := s.repo.ResourceStorageReferenceCount(resource, []string{resource.ID})
	if err != nil {
		return err
	}
	var job *model.ResourceDeletionJob
	if sharedCount == 0 {
		jobs := resourceDeletionJobs(userID, map[string]*model.Resource{resourceStorageIdentity(resource): resource})
		if len(jobs) != 1 {
			return errors.New("无法创建工作流封面删除任务")
		}
		job = &jobs[0]
	}
	if err := s.repo.DiscardCanvasWorkflowCoverDraft(userID, resource, job); err != nil {
		if errors.Is(err, repository.ErrCanvasWorkflowCoverReferenced) {
			return BadAuthRequest("工作流封面已被草稿或发布版本引用，不能删除")
		}
		return err
	}
	return nil
}

func validateCanvasWorkflowCoverUpload(header *multipart.FileHeader) error {
	if header == nil || header.Size <= 0 || header.Size > CanvasWorkflowCoverMaxBytes {
		return BadAuthRequest("工作流封面必须是 10MB 以内的图片")
	}
	file, err := header.Open()
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, CanvasWorkflowCoverMaxBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > CanvasWorkflowCoverMaxBytes {
		return BadAuthRequest("工作流封面内容无法读取")
	}
	detected := strings.ToLower(http.DetectContentType(data[:min(512, len(data))]))
	if detected != "image/png" && detected != "image/jpeg" {
		return BadAuthRequest("裁切后的工作流封面必须是 PNG 或 JPEG")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return BadAuthRequest("工作流封面图片无法解码")
	}
	if config.Width != 1200 || config.Height != 675 {
		return BadAuthRequest("工作流封面必须为裁切后的 1200×675 像素")
	}
	return nil
}
