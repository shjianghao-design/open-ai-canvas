package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestEmbeddedCanvasWorkflowTemplates(t *testing.T) {
	templates, err := loadEmbeddedCanvasWorkflowTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 12 {
		t.Fatalf("embedded templates = %d, want 12", len(templates))
	}
	seen := make(map[string]struct{}, len(templates))
	for _, template := range templates {
		if _, exists := seen[template.Slug]; exists {
			t.Fatalf("duplicate embedded template slug %q", template.Slug)
		}
		seen[template.Slug] = struct{}{}
		if err := validateCanvasWorkflowPayload(template.DraftJSON); err != nil {
			t.Errorf("template %q: %v", template.Slug, err)
		}
	}
}

func TestDecodeCanvasWorkflowReferenceTemplateRejectsInvalidMetadata(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty slug", raw: `{"name":"Name","description":"Description","category":"Category","project":{"nodes":[{"id":"n1"}]}}`},
		{name: "empty project", raw: `{"slug":"slug","name":"Name","description":"Description","category":"Category","project":{"nodes":[]}}`},
		{name: "invalid json", raw: `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeCanvasWorkflowReferenceTemplate(tt.name+".json", []byte(tt.raw)); err == nil {
				t.Fatal("expected invalid template to be rejected")
			}
		})
	}
}

func TestImportCanvasWorkflowProjectRemapsLocalReferencesAndSanitizes(t *testing.T) {
	project := map[string]any{
		"id": "template-project",
		"nodes": []any{
			map[string]any{
				"id": "group", "type": "group", "templateTag": "group", "metadata": map[string]any{
					"marker":            "group",
					"groupChildNodeIds": []any{"child", "orphan"},
					"selectedInputIds":  []any{"child", "external"},
					"references":        []any{"child", "external-ref"},
					"assetBindings":     []any{map[string]any{"nodeId": "child", "role": "style"}},
					"workflowParameters": map[string]any{
						"field:child:prompt": "keep",
						"source:referenceImage": "keep-source-key",
					},
					"prompt":            "保持这段 prompt 原文，不做替换",
					"model":             "image-model",
					"taskId":            "task-old",
					"modelKey":          "model-secret",
					"apiKey":            "api-secret",
				},
			},
			map[string]any{"id": "child", "parentId": "group", "templateTag": "child", "metadata": nil},
			map[string]any{"id": "peer", "templateTag": "peer", "metadata": map[string]any{"marker": "peer", "sourceNodeId": "child", "generationTaskId": "task-old-2", "providerSecret": "secret"}},
		},
		"connections": []any{map[string]any{"id": "edge-old", "fromNodeId": "group", "toNodeId": "child"}},
	}
	beforeNodes := len(project["nodes"].([]any))
	beforeEdges := len(project["connections"].([]any))

	got, err := remapCanvasWorkflowProject(project, "slug", 3)
	if err != nil {
		t.Fatal(err)
	}
	nodes := got["nodes"].([]any)
	if len(nodes) != beforeNodes || len(got["connections"].([]any)) != beforeEdges {
		t.Fatalf("graph counts changed: nodes %d->%d, edges %d->%d", beforeNodes, len(nodes), beforeEdges, len(got["connections"].([]any)))
	}
	byMarker := map[string]map[string]any{}
	newIDs := map[string]bool{}
	for _, raw := range nodes {
		node := raw.(map[string]any)
		byMarker[node["templateTag"].(string)] = node
		newIDs[node["id"].(string)] = true
	}
	group := byMarker["group"]
	child := byMarker["child"]
	peer := byMarker["peer"]
	if child["parentId"] != group["id"] {
		t.Fatalf("parentId was not remapped: %#v", child["parentId"])
	}
	groupMeta := group["metadata"].(map[string]any)
	for _, key := range []string{"groupChildNodeIds", "selectedInputIds"} {
		list := groupMeta[key].([]any)
		if list[0] != child["id"] {
			t.Errorf("%s remap = %#v", key, list)
		}
	}
	if groupMeta["groupChildNodeIds"].([]any)[1] != "orphan" || groupMeta["selectedInputIds"].([]any)[1] != "external" {
		t.Errorf("non-local IDs should remain unchanged: %#v", groupMeta)
	}
	if references := groupMeta["references"].([]any); references[0] != child["id"] || references[1] != "external-ref" {
		t.Errorf("reference fields = %#v", references)
	}
	bindings := groupMeta["assetBindings"].([]any)[0].(map[string]any)
	if bindings["nodeId"] != child["id"] || bindings["role"] != "style" {
		t.Errorf("asset reference fields = %#v", bindings)
	}
	parameters := groupMeta["workflowParameters"].(map[string]any)
	if parameters["field:"+child["id"].(string)+":prompt"] != "keep" || parameters["source:referenceImage"] != "keep-source-key" {
		t.Errorf("workflow parameter references = %#v", parameters)
	}
	peerMeta := peer["metadata"].(map[string]any)
	if peerMeta["sourceNodeId"] != child["id"] {
		t.Errorf("sourceNodeId = %#v", peerMeta["sourceNodeId"])
	}
	if gotPrompt := groupMeta["prompt"]; gotPrompt != "保持这段 prompt 原文，不做替换" {
		t.Errorf("prompt changed: %#v", gotPrompt)
	}
	if groupMeta["model"] != "image-model" {
		t.Errorf("model selection should remain, got %#v", groupMeta["model"])
	}
	if child["metadata"] == nil {
		t.Fatal("nil metadata was not initialized")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"task-old", "model-secret", "api-secret", "providerSecret"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("sensitive/runtime value %q remained in import", forbidden)
		}
	}
	if !newIDs[group["id"].(string)] {
		t.Fatal("unexpected project identity fields")
	}
	edge := got["connections"].([]any)[0].(map[string]any)
	if edge["id"] == "edge-old" || edge["fromNodeId"] != group["id"] || edge["toNodeId"] != child["id"] {
		t.Fatalf("edge identity/reference not remapped: %#v", edge)
	}
}

func TestValidateCanvasWorkflowPayloadAllowsUpTo200Nodes(t *testing.T) {
	for _, count := range []int{41, 200} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			nodes := make([]map[string]any, count)
			for i := range nodes {
				nodes[i] = map[string]any{"id": fmt.Sprintf("n-%d", i), "metadata": map[string]any{"prompt": "text"}}
			}
			payload, err := json.Marshal(map[string]any{"nodes": nodes, "connections": []any{}})
			if err != nil {
				t.Fatal(err)
			}
			if err := validateCanvasWorkflowPayload(string(payload)); err != nil {
				t.Fatalf("%d-node template rejected: %v", count, err)
			}
		})
	}
	for _, count := range []int{0, 201} {
		t.Run(fmt.Sprintf("reject-%d", count), func(t *testing.T) {
			nodes := make([]map[string]any, count)
			payload, err := json.Marshal(map[string]any{"nodes": nodes})
			if err != nil {
				t.Fatal(err)
			}
			if err := validateCanvasWorkflowPayload(string(payload)); err == nil {
				t.Fatalf("%d-node template should be rejected", count)
			}
		})
	}
}
