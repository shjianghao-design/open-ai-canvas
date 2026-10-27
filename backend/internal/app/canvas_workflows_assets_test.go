package app

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// 内置模板的配图通过 /workflow-samples-reference/...  这类静态路径引用，
// 由 nginx 直接托管 web/public 下的实体文件。这些素材曾经在合并时被当成
// 「仅测试数据」排除出仓库，结果线上模板卡片与详情页整片空白，而构建和
// 单元测试全都通过。此用例把「模板引用」与「磁盘实体」绑死，避免复发。
var canvasWorkflowSampleRefPattern = regexp.MustCompile(`/workflow-samples-reference[0-9]*/[^"\\\s,)]*`)

func TestEmbeddedCanvasWorkflowTemplateAssetsExist(t *testing.T) {
	templates, err := loadEmbeddedCanvasWorkflowTemplates()
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	// backend/internal/app -> 仓库根
	publicDir := filepath.Join(filepath.Dir(file), "..", "..", "..", "web", "public")
	if info, err := os.Stat(publicDir); err != nil || !info.IsDir() {
		// 只跑后端编译产物的上下文（例如单独 vendor 的模块）里没有前端目录，跳过比对。
		t.Skipf("找不到前端静态目录 %s，跳过素材比对", publicDir)
	}

	total := 0
	missing := make([]string, 0)
	for _, template := range templates {
		for _, ref := range canvasWorkflowSampleRefPattern.FindAllString(template.DraftJSON, -1) {
			total++
			local := filepath.Join(publicDir, filepath.FromSlash(strings.TrimPrefix(ref, "/")))
			info, err := os.Stat(local)
			if err != nil || info.IsDir() {
				missing = append(missing, template.Slug+" -> "+ref)
				continue
			}
			if info.Size() == 0 {
				missing = append(missing, template.Slug+" -> "+ref+"（零字节）")
			}
		}
	}
	if total == 0 {
		t.Fatal("内置模板没有引用任何 sample 素材，引用方式可能已变更")
	}
	for _, item := range missing {
		t.Errorf("模板引用的素材不存在: %s", item)
	}
	if len(missing) > 0 {
		t.Fatalf("%d 处素材引用无法在 web/public 下找到实体，共 %d 处引用", len(missing), total)
	}
}
