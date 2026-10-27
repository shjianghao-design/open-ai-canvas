package canvas

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"yingce/backend/internal/assets"
	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/gorm"
)

const canvasHistoryLimit = 20
const canvasHistoryInterval = 5 * time.Minute

// 浏览器、云端 Agent 和创作任务共用此入口；外层事务仍负责任务和画布的一致性。
func SaveDocumentWithHistory(repo *repository.Repository, before *model.CanvasProject, after *model.CanvasProject, reason string) error {
	snapshot, resourceIDs, err := buildCanvasSnapshot(before, *after, reason)
	if err != nil {
		return err
	}
	refs := map[string]struct{}{}
	if err := assets.CollectOwnedDocumentReferences(after.PayloadJSON, refs); err != nil {
		return err
	}
	currentResourceIDs := assets.SortedIDs(refs)
	if before != nil {
		// 与素材守卫同一口径：预镜像里已经失效的资源属于历史遗留损坏。素材被硬删除后客户端
		// 没有入口摘掉节点上的旧引用，若继续要求它存在，画布在提交阶段会再次被拦下，等于把
		// 画布永久锁死；指向空资源的快照行也没有任何用途。只放行「预镜像已引用」的 ID，
		// 本次变更新引入的缺失引用仍然会在这里被拒绝。
		previous := map[string]struct{}{}
		if err := assets.CollectOwnedDocumentReferences(before.PayloadJSON, previous); err != nil {
			return err
		}
		damaged, err := missingCanvasResourceIDs(repo, before.UserID, assets.SortedIDs(previous))
		if err != nil {
			return err
		}
		if len(damaged) > 0 {
			damagedSet := make(map[string]struct{}, len(damaged))
			for _, id := range damaged {
				damagedSet[id] = struct{}{}
			}
			currentResourceIDs = slices.DeleteFunc(currentResourceIDs, func(id string) bool {
				_, missing := damagedSet[id]
				return missing
			})
			resourceIDs = slices.DeleteFunc(resourceIDs, func(id string) bool {
				_, missing := damagedSet[id]
				return missing
			})
		}
	}
	return repo.SaveCanvasWithSnapshot(after, snapshot, resourceIDs, currentResourceIDs, after.UpdatedAt.Add(-canvasHistoryInterval), canvasHistoryLimit, reason == "before_restore" || reason == "before_resource_repair")
}

func missingCanvasResourceIDs(repo *repository.Repository, userID string, resourceIDs []string) ([]string, error) {
	if len(resourceIDs) == 0 {
		return nil, nil
	}
	resources, err := repo.ResourcesForUserIDs(userID, resourceIDs)
	if err != nil {
		return nil, err
	}
	ready := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if resource.Status == model.ResourceStatusReady {
			ready[resource.ID] = struct{}{}
		}
	}
	missing := make([]string, 0, len(resourceIDs))
	for _, id := range resourceIDs {
		if _, exists := ready[id]; !exists {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// Explicit repair keeps the damaged preimage for audit/undo, but only protects
// resources that still exist. The incoming document remains strictly validated.
func (s *Service) RepairUserCanvasProject(userID string, raw json.RawMessage) (UserDataSummary, error) {
	return s.upsertUserCanvasProjectWithHistory(userID, raw, "before_resource_repair")
}

func canvasRevisionConflict() error {
	return kernel.NewAppError(http.StatusConflict, "云端画布已有更新，未确认操作会保留并按原请求自动重放")
}

type CanvasHistoryList struct {
	Snapshots       []model.CanvasSnapshot `json:"snapshots"`
	CurrentRevision int64                  `json:"currentRevision"`
}

func (s *Service) scopedCanvasHistoryProject(userID string, canvasID string) (*model.CanvasProject, error) {
	project, err := s.repo.CanvasProjectMetadata(userID, strings.TrimSpace(canvasID))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NewAppError(http.StatusNotFound, "画布不存在或无权访问")
	}
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (s *Service) CanvasHistory(userID string, canvasID string) (CanvasHistoryList, error) {
	project, err := s.scopedCanvasHistoryProject(userID, canvasID)
	if err != nil {
		return CanvasHistoryList{}, err
	}
	items, err := s.repo.CanvasSnapshots(project.UserID, project.ID, canvasHistoryLimit)
	return CanvasHistoryList{Snapshots: items, CurrentRevision: project.Revision}, err
}

func (s *Service) CanvasHistorySnapshot(userID string, canvasID, snapshotID string) (*model.CanvasSnapshot, error) {
	project, err := s.scopedCanvasHistoryProject(userID, canvasID)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.CanvasSnapshot(project.UserID, project.ID, snapshotID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NewAppError(http.StatusNotFound, "历史版本不存在或已过期，请刷新历史列表")
	}
	return item, err
}

func (s *Service) RestoreCanvasHistory(userID string, canvasID, snapshotID string, revision *int64) (UserDataSummary, error) {
	if revision == nil {
		return UserDataSummary{}, kernel.NewAppError(http.StatusPreconditionRequired, "请先刷新画布版本再恢复")
	}
	current, err := s.scopedCanvasHistoryProject(userID, canvasID)
	if err != nil {
		return UserDataSummary{}, err
	}
	if current.Revision != *revision {
		return UserDataSummary{}, canvasRevisionConflict()
	}
	item, err := s.CanvasHistorySnapshot(userID, canvasID, snapshotID)
	if err != nil {
		return UserDataSummary{}, err
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(item.PayloadJSON), &payload); err != nil {
		return UserDataSummary{}, err
	}
	// Restore document content while retaining its present ownership and business association.
	for key, value := range map[string]any{
		"id": current.ID, "revision": *revision, "createdAt": current.CreatedAt,
		"projectId": current.ProjectID,
	} {
		payload[key], _ = json.Marshal(value)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return UserDataSummary{}, err
	}
	return s.upsertUserCanvasProjectWithHistory(userID, raw, "before_restore")
}

func canvasHistoryContent(project model.CanvasProject) ([]byte, error) {
	raw, err := canvasProjectPayload(project)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	for _, key := range []string{"revision", "updatedAt", "createdAt", "viewport", "remoteContentHash"} {
		delete(payload, key)
	}
	return json.Marshal(payload)
}

func buildCanvasSnapshot(before *model.CanvasProject, after model.CanvasProject, reason string) (*model.CanvasSnapshot, []string, error) {
	if before == nil {
		return nil, nil, nil
	}
	oldContent, err := canvasHistoryContent(*before)
	if err != nil {
		return nil, nil, err
	}
	newContent, err := canvasHistoryContent(after)
	if err != nil {
		return nil, nil, err
	}
	if reason != "before_restore" && bytes.Equal(oldContent, newContent) {
		return nil, nil, nil
	}
	raw, err := canvasProjectPayload(*before)
	if err != nil {
		return nil, nil, err
	}
	var counts struct {
		Nodes       []json.RawMessage
		Connections []json.RawMessage
	}
	if err := json.Unmarshal(raw, &counts); err != nil {
		return nil, nil, err
	}
	resources := map[string]struct{}{}
	if err := assets.CollectOwnedDocumentReferences(string(raw), resources); err != nil {
		return nil, nil, err
	}
	return &model.CanvasSnapshot{
		ID: kernel.NewID(), CanvasID: before.ID, UserID: before.UserID, Revision: before.Revision,
		Title: before.Title, NodeCount: len(counts.Nodes), ConnectionCount: len(counts.Connections),
		PayloadJSON: string(raw), PayloadBytes: len(raw), Reason: reason,
		ContentUpdatedAt: before.UpdatedAt, CreatedAt: after.UpdatedAt,
	}, assets.SortedIDs(resources), nil
}

// canvasPreviousPayload 返回库内既有画布文档，供素材守卫区分「历史遗留的失效引用」和
// 「本次变更新引入的失效引用」。
func canvasPreviousPayload(project *model.CanvasProject) json.RawMessage {
	if project == nil || project.PayloadJSON == "" {
		return nil
	}
	return json.RawMessage(project.PayloadJSON)
}

// Row metadata is authoritative, including changes made by project association operations.
func canvasProjectPayload(project model.CanvasProject) (json.RawMessage, error) {
	metadata := []struct {
		key   string
		value any
	}{
		{key: "id", value: project.ID},
		{key: "title", value: project.Title},
		{key: "projectId", value: project.ProjectID},
		{key: "revision", value: project.Revision},
		{key: "createdAt", value: project.CreatedAt},
		{key: "updatedAt", value: project.UpdatedAt},
	}
	replacements := make(map[string]json.RawMessage, len(metadata))
	for _, item := range metadata {
		encoded, err := json.Marshal(item.value)
		if err != nil {
			return nil, err
		}
		replacements[item.key] = encoded
	}

	decoder := json.NewDecoder(strings.NewReader(project.PayloadJSON))
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if opening != json.Delim('{') {
		return nil, kernel.BadAuthRequest("画布数据格式错误")
	}

	result := make([]byte, 0, len(project.PayloadJSON)+128)
	result = append(result, '{')
	written := make(map[string]struct{}, len(replacements))
	first := true
	appendField := func(key string, value json.RawMessage) error {
		if !first {
			result = append(result, ',')
		}
		first = false
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return err
		}
		result = append(result, encodedKey...)
		result = append(result, ':')
		result = append(result, value...)
		return nil
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, kernel.BadAuthRequest("画布数据格式错误")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if replacement, exists := replacements[key]; exists {
			if _, duplicate := written[key]; duplicate {
				continue
			}
			value = replacement
			written[key] = struct{}{}
		}
		if err := appendField(key, value); err != nil {
			return nil, err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if closing != json.Delim('}') {
		return nil, kernel.BadAuthRequest("画布数据格式错误")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, kernel.BadAuthRequest("画布数据格式错误")
		}
		return nil, err
	}
	for _, item := range metadata {
		if _, exists := written[item.key]; exists {
			continue
		}
		if err := appendField(item.key, replacements[item.key]); err != nil {
			return nil, err
		}
	}
	result = append(result, '}')
	return json.RawMessage(result), nil
}
