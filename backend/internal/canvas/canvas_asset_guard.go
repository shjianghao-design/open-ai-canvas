package canvas

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"yingce/backend/internal/assets"
	"yingce/backend/internal/kernel"

	"yingce/backend/internal/model"
)

type MediaAssetReference struct {
	AssetID       string
	ResourceID    string
	NodeID        string
	Path          string
	ReferenceType string
}

// validateCanvasMediaAssets is the final server-side invariant for canvas sync:
// a persisted canvas may only point at an uploaded Resource through an Asset
// owned by the same user. The client writes Assets before canvases, so rejecting
// an incomplete pair prevents a durable Resource-only (ghost capacity) state.
func (s *Service) ValidateCanvasMediaAssets(userID string, raw json.RawMessage) error {
	return s.ValidateCanvasMediaAssetsWithPrevious(userID, raw, nil)
}

// ValidateCanvasMediaAssetsWithPrevious 以库内既有文档为基线执行同一套守卫，只拦截本次
// 变更新引入的失效引用。资源被硬删除后客户端没有任何入口摘掉节点上的旧引用，严格校验
// 无法阻止已经发生的损坏，只会让整张画布在之后每一次编辑都 409（节点新建、连线、改标题
// 全部失败）。previous 为空时等同于严格模式，新建画布和显式修复仍按严格规则处理。
func (s *Service) ValidateCanvasMediaAssetsWithPrevious(userID string, raw, previous json.RawMessage) error {
	grandfatheredResources, grandfatheredBindings := canvasGrandfatheredReferences(previous)
	references, err := MediaAssetReferences(raw)
	if err != nil {
		return kernel.BadAuthRequest("画布媒体数据格式错误")
	}
	allResourceReferences, err := assets.CollectDocumentResourceReferences(string(raw))
	if err != nil {
		return kernel.BadAuthRequest("画布媒体数据格式错误")
	}
	if len(references) == 0 && len(allResourceReferences) == 0 {
		return nil
	}

	assetIDSet := make(map[string]struct{}, len(references))
	resourceIDSet := make(map[string]struct{}, len(references)+len(allResourceReferences))
	for _, reference := range references {
		if reference.ResourceID != "" {
			resourceIDSet[reference.ResourceID] = struct{}{}
		}
		if reference.AssetID == "" {
			if _, grandfathered := grandfatheredBindings[canvasBindingKey(reference)]; !grandfathered {
				return canvasMediaAssetsPendingError()
			}
			continue
		}
		assetIDSet[reference.AssetID] = struct{}{}
	}
	for _, reference := range allResourceReferences {
		resourceIDSet[reference.ResourceID] = struct{}{}
	}

	ownedAssets, err := s.repo.AssetsForUserIDs(userID, assets.SortedIDs(assetIDSet))
	if err != nil {
		return err
	}
	assetResources := make(map[string]map[string]struct{}, len(ownedAssets))
	for _, asset := range ownedAssets {
		assetResources[asset.ID] = assets.DocumentReferencedIDs(asset.PayloadJSON, resourceIDSet)
	}
	history, err := s.projectImageHistory(userID, ownedAssets)
	if err != nil {
		return err
	}
	for assetID, ids := range history {
		for resourceID := range ids {
			assetResources[assetID][resourceID] = struct{}{}
		}
	}

	resources, err := s.repo.ResourcesForUserIDs(userID, assets.SortedIDs(resourceIDSet))
	if err != nil {
		return err
	}
	readyResources := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if resource.Status == model.ResourceStatusReady {
			readyResources[resource.ID] = struct{}{}
		}
	}
	missing := make([]assets.DocumentResourceReference, 0)
	for _, reference := range allResourceReferences {
		if _, exists := readyResources[reference.ResourceID]; !exists {
			// 预镜像已经引用过同一资源：损坏发生在这次写入之前，放行以免画布被永久锁死。
			if _, grandfathered := grandfatheredResources[reference.ResourceID]; grandfathered {
				continue
			}
			reference.Source = "current"
			missing = append(missing, reference)
		}
	}
	if len(missing) > 0 {
		return canvasResourcesMissingError(missing)
	}

	for _, reference := range references {
		if _, grandfathered := grandfatheredBindings[canvasBindingKey(reference)]; grandfathered {
			continue
		}
		resourceIDs, assetExists := assetResources[reference.AssetID]
		if !assetExists {
			return canvasMediaAssetsPendingError()
		}
		if _, matches := resourceIDs[reference.ResourceID]; !matches {
			return canvasMediaAssetsPendingError()
		}
	}
	return nil
}

// canvasGrandfatheredReferences 取出库内既有文档里已经存在的失效线索：按资源 ID 记录，
// 以及按「素材 + 资源」绑定记录。media 节点上的 assetId 可能同样指向已被删除的素材行，
// 只放行资源而不放行绑定会让这类画布继续卡在「尚未进入素材库」上。
func canvasGrandfatheredReferences(previous json.RawMessage) (map[string]struct{}, map[string]struct{}) {
	resources := map[string]struct{}{}
	bindings := map[string]struct{}{}
	if len(previous) == 0 {
		return resources, bindings
	}
	if references, err := assets.CollectDocumentResourceReferences(string(previous)); err == nil {
		for _, reference := range references {
			resources[reference.ResourceID] = struct{}{}
		}
	}
	if references, err := MediaAssetReferences(previous); err == nil {
		for _, reference := range references {
			bindings[canvasBindingKey(reference)] = struct{}{}
		}
	}
	return resources, bindings
}

func canvasBindingKey(reference MediaAssetReference) string {
	return reference.AssetID + "\x00" + reference.ResourceID
}

func canvasMediaAssetsPendingError() *kernel.AppError {
	err := kernel.BadAuthRequest("画布媒体尚未进入素材库，正在重新对齐")
	err.Reason = "canvas_media_assets_pending"
	return err
}

func canvasResourcesMissingError(references []assets.DocumentResourceReference) *kernel.AppError {
	ids := make(map[string]struct{}, len(references))
	for _, reference := range references {
		ids[reference.ResourceID] = struct{}{}
	}
	err := kernel.NewAppError(http.StatusConflict, "画布引用的素材已变化，当前内容未被覆盖，请修复缺失素材后重试")
	err.Code = kernel.CodeCanvasResourcesMissing
	err.Reason = kernel.ReasonCanvasResourcesMissing
	err.Details = map[string]any{"resourceIds": assets.SortedIDs(ids), "missingResources": references}
	return err
}

// validateAssetCanvasReferences prevents an Asset update from changing the
// resource behind a canvas that already points at that Asset.
func (s *Service) ValidateAssetCanvasReferences(userID string, asset model.Asset) error {
	history, err := s.projectImageHistory(userID, []model.Asset{asset})
	if err != nil {
		return err
	}
	canvases, err := s.repo.CanvasProjects(userID)
	if err != nil {
		return err
	}
	for _, canvas := range canvases {
		references, parseErr := MediaAssetReferences(json.RawMessage(canvas.PayloadJSON))
		if parseErr != nil {
			return kernel.BadAuthRequest("已有画布媒体数据无法解析，已停止修改素材")
		}
		for _, reference := range references {
			if reference.AssetID != asset.ID {
				continue
			}
			candidate := map[string]struct{}{reference.ResourceID: {}}
			_, historical := history[asset.ID][reference.ResourceID]
			if !assets.DocumentReferences(asset.PayloadJSON, candidate) && !historical {
				return kernel.BadAuthRequest("素材仍被画布引用，不能替换为其他云端资源")
			}
		}
	}
	return nil
}

// validateAssetReplacementCanvasReferences applies the same invariant to the
// legacy full-replacement endpoint, which otherwise could silently remove an
// Asset that a canvas still needs.
func (s *Service) ValidateAssetReplacementCanvasReferences(userID string, replacement []model.Asset) error {
	history, err := s.projectImageHistory(userID, replacement)
	if err != nil {
		return err
	}
	assetByID := make(map[string]model.Asset, len(replacement))
	for _, asset := range replacement {
		assetByID[asset.ID] = asset
	}
	canvases, err := s.repo.CanvasProjects(userID)
	if err != nil {
		return err
	}
	for _, canvas := range canvases {
		references, parseErr := MediaAssetReferences(json.RawMessage(canvas.PayloadJSON))
		if parseErr != nil {
			return kernel.BadAuthRequest("已有画布媒体数据无法解析，已停止替换素材库")
		}
		for _, reference := range references {
			asset, exists := assetByID[reference.AssetID]
			if !exists {
				return kernel.BadAuthRequest("素材仍被画布引用，不能从素材库移除")
			}
			candidate := map[string]struct{}{reference.ResourceID: {}}
			_, historical := history[asset.ID][reference.ResourceID]
			if !assets.DocumentReferences(asset.PayloadJSON, candidate) && !historical {
				return kernel.BadAuthRequest("画布媒体与替换后的素材库记录不一致")
			}
		}
	}
	return nil
}

// 场景、道具换图后保留旧画布的不可变资源引用；信任版本表，不信任客户端声明的历史。
func (s *Service) projectImageHistory(userID string, candidates []model.Asset) (map[string]map[string]struct{}, error) {
	ids := []string{}
	for _, asset := range candidates {
		if asset.Category == model.AssetCategoryEnvironment || asset.Category == model.AssetCategoryProp {
			ids = append(ids, asset.ID)
		}
	}
	references, err := s.repo.ProjectAssetImageReferences(userID, ids)
	if err != nil {
		return nil, err
	}
	history := map[string]map[string]struct{}{}
	for _, reference := range references {
		if history[reference.AssetID] == nil {
			history[reference.AssetID] = map[string]struct{}{}
		}
		history[reference.AssetID][reference.ResourceID] = struct{}{}
	}
	return history, nil
}

func MediaAssetReferences(raw json.RawMessage) ([]MediaAssetReference, error) {
	var payload struct {
		Nodes []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Metadata struct {
				AssetID    string `json:"assetId"`
				StorageKey string `json:"storageKey"`
				Content    string `json:"content"`
			} `json:"metadata"`
		} `json:"nodes"`
		Timeline struct {
			Clips []struct {
				DirectMedia *struct {
					Kind       string `json:"kind"`
					AssetID    string `json:"assetId"`
					StorageKey string `json:"storageKey"`
					URL        string `json:"url"`
					DataURL    string `json:"dataUrl"`
					Content    string `json:"content"`
				} `json:"directMedia"`
			} `json:"clips"`
		} `json:"timeline"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	references := make([]MediaAssetReference, 0)
	for _, node := range payload.Nodes {
		if !isCanvasMediaKind(node.Type) {
			continue
		}
		assetID := strings.TrimSpace(node.Metadata.AssetID)
		resourceID := firstCanvasResourceID(node.Metadata.StorageKey, node.Metadata.Content)
		if resourceID != "" {
			references = append(references, MediaAssetReference{
				AssetID: assetID, ResourceID: resourceID, NodeID: node.ID,
				Path: "nodes[" + node.ID + "].metadata.storageKey", ReferenceType: "storageKey",
			})
		}
	}
	for index, clip := range payload.Timeline.Clips {
		media := clip.DirectMedia
		if media == nil || !isCanvasMediaKind(media.Kind) {
			continue
		}
		resourceID := firstCanvasResourceID(media.StorageKey, media.URL, media.DataURL, media.Content)
		if resourceID == "" {
			continue
		}
		references = append(references, MediaAssetReference{
			AssetID: strings.TrimSpace(media.AssetID), ResourceID: resourceID,
			Path: "timeline.clips[" + strconv.Itoa(index) + "].directMedia", ReferenceType: "directMedia",
		})
	}
	return references, nil
}

func firstCanvasResourceID(values ...string) string {
	for _, value := range values {
		if resourceID := assets.ResourceID(value); resourceID != "" {
			return resourceID
		}
	}
	return ""
}

func isCanvasMediaKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image", "video", "audio":
		return true
	default:
		return false
	}
}
