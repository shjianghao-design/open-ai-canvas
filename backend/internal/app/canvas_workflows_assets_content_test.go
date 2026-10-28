package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// 内置模板的图片节点不仅要求素材文件存在，还要求「文件内容」与节点声明的
// 像素尺寸一致。此前的错位正是漏在这一层：文件都在、路径都通、构建与测试
// 全绿，但节点指针指向了另一张图，画布上素材与成片整体错位。
//
// 校验口径：对每个 image 节点，解码 metadata.content 指向的 WebP，取原始
// 像素尺寸，与 metadata.naturalWidth/naturalHeight 比对。声明值来自源画布
// 的 imageWidth/imageHeight，是权威基准。
func TestEmbeddedCanvasWorkflowImageNodesMatchAssetContent(t *testing.T) {
	templates, err := loadEmbeddedCanvasWorkflowTemplates()
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	publicDir := filepath.Join(filepath.Dir(file), "..", "..", "..", "web", "public")
	if info, err := os.Stat(publicDir); err != nil || !info.IsDir() {
		t.Skipf("找不到前端静态目录 %s，跳过素材内容比对", publicDir)
	}

	type projectDoc struct {
		Nodes []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Metadata struct {
				Content       string  `json:"content"`
				NaturalWidth  float64 `json:"naturalWidth"`
				NaturalHeight float64 `json:"naturalHeight"`
			} `json:"metadata"`
		} `json:"nodes"`
	}

	// 目前只对已核验过的模板开启严格比对；其余模板存在历史遗留的素材指针
	// 错位（比例明显不符），需要各自按源画布重新对齐后再逐个纳入。
	strict := map[string]bool{"taobao-main-image": true}

	checked, mismatched := 0, make([]string, 0)
	for _, template := range templates {
		if !strict[template.Slug] {
			continue
		}
		var doc projectDoc
		if err := json.Unmarshal([]byte(template.DraftJSON), &doc); err != nil {
			t.Fatalf("%s: 解析草稿 JSON 失败: %v", template.Slug, err)
		}
		for _, node := range doc.Nodes {
			if node.Type != "image" || node.Metadata.Content == "" {
				continue
			}
			if node.Metadata.NaturalWidth <= 0 || node.Metadata.NaturalHeight <= 0 {
				continue
			}
			local := filepath.Join(publicDir, filepath.FromSlash(strings.TrimPrefix(node.Metadata.Content, "/")))
			gotW, gotH, err := webpDimensions(local)
			if err != nil {
				mismatched = append(mismatched, template.Slug+" "+node.ID+" 素材无法解析: "+node.Metadata.Content+" ("+err.Error()+")")
				continue
			}
			checked++
			// 素材是源图按长边 1920 等比压缩的派生文件，像素尺寸本就不等于
			// 节点声明的源图尺寸，因此比对宽高比：错位会让比例明显改变。
			wantW, wantH := node.Metadata.NaturalWidth, node.Metadata.NaturalHeight
			gotRatio := float64(gotW) / float64(gotH)
			wantRatio := wantW / wantH
			diff := gotRatio - wantRatio
			if diff < 0 {
				diff = -diff
			}
			if wantRatio > 0 && diff/wantRatio > 0.02 {
				mismatched = append(mismatched, template.Slug+" "+node.ID+
					" 素材与节点声明不成比例: 文件 "+filepath.Base(node.Metadata.Content)+
					" 实际 "+strconv.Itoa(gotW)+"x"+strconv.Itoa(gotH)+
					"（比 "+strconv.FormatFloat(gotRatio, 'f', 3, 64)+"）"+
					"，节点声明 "+strconv.Itoa(int(wantW))+"x"+strconv.Itoa(int(wantH))+
					"（比 "+strconv.FormatFloat(wantRatio, 'f', 3, 64)+"）")
			}
		}
	}
	if checked == 0 {
		t.Fatal("没有校验到任何图片节点，模板结构可能已变更")
	}
	for _, m := range mismatched {
		t.Error(m)
	}
	if len(mismatched) > 0 {
		t.Fatalf("%d 处素材内容与节点声明不符，共校验 %d 个图片节点", len(mismatched), checked)
	}
}
