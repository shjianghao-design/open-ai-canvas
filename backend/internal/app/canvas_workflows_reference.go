package app

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

// The reference templates are authored as ordinary JSON so the catalog can be
// reviewed and updated without duplicating large project literals in Go.
//
//go:embed workflow_templates/*.json
var embeddedCanvasWorkflowTemplates embed.FS

type canvasWorkflowReferenceTemplate struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Project     json.RawMessage `json:"project"`
}

func loadEmbeddedCanvasWorkflowTemplates() ([]model.CanvasWorkflowTemplate, error) {
	return canvasWorkflowTemplatesFromFS(embeddedCanvasWorkflowTemplates)
}

func canvasWorkflowTemplatesFromFS(templateFS fs.FS) ([]model.CanvasWorkflowTemplate, error) {
	paths, err := fs.Glob(templateFS, "workflow_templates/*.json")
	if err != nil {
		return nil, fmt.Errorf("枚举内置工作流模板失败: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("未找到内置工作流模板")
	}
	sort.Strings(paths)
	result := make([]model.CanvasWorkflowTemplate, 0, len(paths))
	for _, path := range paths {
		payload, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return nil, fmt.Errorf("读取内置工作流模板 %s 失败: %w", path, err)
		}
		item, err := decodeCanvasWorkflowReferenceTemplate(path, payload)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func decodeCanvasWorkflowReferenceTemplate(path string, raw []byte) (model.CanvasWorkflowTemplate, error) {
	var source canvasWorkflowReferenceTemplate
	if err := json.Unmarshal(raw, &source); err != nil {
		return model.CanvasWorkflowTemplate{}, fmt.Errorf("内置工作流模板 %s 不是合法 JSON: %w", path, err)
	}
	source.Slug = strings.TrimSpace(source.Slug)
	source.Name = strings.TrimSpace(source.Name)
	source.Description = strings.TrimSpace(source.Description)
	source.Category = strings.TrimSpace(source.Category)
	if source.Slug == "" || source.Name == "" || source.Category == "" || len(source.Project) == 0 || string(source.Project) == "null" {
		return model.CanvasWorkflowTemplate{}, fmt.Errorf("内置工作流模板 %s 缺少 slug/name/category/project", path)
	}
	var project map[string]any
	if err := json.Unmarshal(source.Project, &project); err != nil || project == nil {
		if err == nil {
			err = fmt.Errorf("project 不是对象")
		}
		return model.CanvasWorkflowTemplate{}, fmt.Errorf("内置工作流模板 %s 的 project 无效: %w", path, err)
	}
	encodedProject, err := json.Marshal(project)
	if err != nil {
		return model.CanvasWorkflowTemplate{}, fmt.Errorf("内置工作流模板 %s 的 project 编码失败: %w", path, err)
	}
	if err := validateCanvasWorkflowPayload(string(encodedProject)); err != nil {
		return model.CanvasWorkflowTemplate{}, fmt.Errorf("内置工作流模板 %s 校验失败: %w", path, err)
	}
	return model.CanvasWorkflowTemplate{
		ID:          "builtin-" + source.Slug,
		Slug:        source.Slug,
		Name:        source.Name,
		Description: source.Description,
		Category:    source.Category,
		Version:     1,
		DraftJSON:   string(encodedProject),
		Published:   true,
		Enabled:     true,
		Builtin:     true,
		Revision:    1,
	}, nil
}

var canvasWorkflowLocalReferenceKeys = map[string]bool{
	"parentid":              true,
	"nodeid":                true,
	"sourcenodeid":          true,
	"targetnodeid":          true,
	"fromnodeid":            true,
	"tonodeid":              true,
	"batchrootid":           true,
	"primaryimageid":        true,
	"selectedinputids":      true,
	"referenceids":          true,
	"references":            true,
	"assetbindings":         true,
	"referenceassetnodeids": true,
}

var canvasWorkflowSensitiveKeys = map[string]bool{
	"apikey": true, "modelkey": true, "storagekey": true,
	"taskid": true, "generationtaskid": true, "planningtaskid": true,
	"extractiontaskid": true, "rejectedtaskid": true, "stagetaskids": true,
	"taskstatus": true, "taskprogress": true, "taskstage": true,
	"taskcreatedat": true, "taskupdatedat": true, "generationbatches": true,
	"agentdraftrunid": true, "errordetails": true,
}

func isCanvasWorkflowSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	if canvasWorkflowSensitiveKeys[normalized] {
		return true
	}
	return strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") || strings.Contains(normalized, "credential") || strings.HasSuffix(normalized, "token")
}

func isCanvasWorkflowLocalReferenceKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	if canvasWorkflowLocalReferenceKeys[normalized] {
		return true
	}
	return strings.HasSuffix(normalized, "nodeid") || strings.HasSuffix(normalized, "nodeids")
}

func remapCanvasWorkflowProject(source map[string]any, slug string, version int) (map[string]any, error) {
	if source == nil {
		return nil, fmt.Errorf("工作流模板 project 为空")
	}
	rawNodes, ok := source["nodes"].([]any)
	if !ok || len(rawNodes) == 0 {
		return nil, fmt.Errorf("工作流模板 project 节点无效")
	}
	ids := make(map[string]string, len(rawNodes))
	for _, raw := range rawNodes {
		node, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("工作流模板包含无效节点")
		}
		oldID := kernel.StringValue(node["id"])
		if oldID == "" {
			return nil, fmt.Errorf("工作流模板节点缺少 id")
		}
		if _, exists := ids[oldID]; exists {
			return nil, fmt.Errorf("工作流模板节点 id 重复: %s", oldID)
		}
		ids[oldID] = kernel.NewID()
	}

	cloned, ok := cloneCanvasWorkflowForImport(source).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("工作流模板 project 克隆失败")
	}
	remapCanvasWorkflowValue(cloned, "", ids, false)
	clonedNodes, ok := cloned["nodes"].([]any)
	if !ok || len(clonedNodes) != len(rawNodes) {
		return nil, fmt.Errorf("工作流模板节点克隆失败")
	}
	for index, raw := range clonedNodes {
		node := raw.(map[string]any)
		oldID := kernel.StringValue(rawNodes[index].(map[string]any)["id"])
		node["id"] = ids[oldID]
		metadata, ok := node["metadata"].(map[string]any)
		if !ok || metadata == nil {
			metadata = map[string]any{}
			node["metadata"] = metadata
		}
		metadata["status"] = "idle"
		metadata["workflowTemplateId"] = slug
		metadata["workflowTemplateVersion"] = version
	}
	if rawConnections, exists := cloned["connections"]; exists {
		connections, ok := rawConnections.([]any)
		if !ok {
			return nil, fmt.Errorf("工作流模板 connections 无效")
		}
		for _, raw := range connections {
			edge, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("工作流模板包含无效连接")
			}
			edge["id"] = kernel.NewID()
			for _, key := range []string{"fromNodeId", "toNodeId"} {
				newID := kernel.StringValue(edge[key])
				if newID != "" && !canvasWorkflowNewIDExists(ids, newID) {
					return nil, fmt.Errorf("工作流模板连接引用未知节点: %s", newID)
				}
			}
		}
	}
	return cloned, nil
}

func canvasWorkflowNewIDExists(ids map[string]string, candidate string) bool {
	for _, newID := range ids {
		if newID == candidate {
			return true
		}
	}
	return false
}

func cloneCanvasWorkflowForImport(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			if isCanvasWorkflowSensitiveKey(key) {
				continue
			}
			result[key] = cloneCanvasWorkflowForImport(child)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for index, child := range item {
			result[index] = cloneCanvasWorkflowForImport(child)
		}
		return result
	default:
		return value
	}
}

func remapCanvasWorkflowValue(value any, parentKey string, ids map[string]string, inImportSource bool) any {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			newKey := remapCanvasWorkflowMapKey(key, parentKey, ids)
			if newKey != key {
				delete(item, key)
			}
			childInImportSource := inImportSource || strings.EqualFold(newKey, "importSource")
			item[newKey] = remapCanvasWorkflowValue(child, newKey, ids, childInImportSource)
		}
	case []any:
		for index, child := range item {
			item[index] = remapCanvasWorkflowValue(child, parentKey, ids, inImportSource)
		}
	case string:
		if !inImportSource && isCanvasWorkflowLocalReferenceKey(parentKey) {
			if newID, ok := ids[item]; ok {
				return newID
			}
		}
	}
	return value
}

func remapCanvasWorkflowMapKey(key, parentKey string, ids map[string]string) string {
	if !strings.EqualFold(parentKey, "workflowParameters") || !strings.HasPrefix(key, "field:") {
		return key
	}
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 {
		return key
	}
	if newID, ok := ids[parts[1]]; ok {
		return "field:" + newID + ":" + parts[2]
	}
	return key
}
