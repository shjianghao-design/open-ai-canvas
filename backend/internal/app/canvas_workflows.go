package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

const ecommerceCanvasWorkflowSlug = "ecommerce-product-detail-suite"

type CanvasWorkflowTemplateSummary struct {
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Version     int       `json:"version"`
	Builtin     bool      `json:"builtin"`
	NodeCount   int       `json:"nodeCount"`
	MediaTypes  []string  `json:"mediaTypes"`
	CoverURL    string    `json:"coverUrl,omitempty"`
	ImportTotal int64     `json:"importTotal"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CanvasWorkflowTemplateDetail struct {
	CanvasWorkflowTemplateSummary
	// TemplateID / LatestVersionID 供前台记录「本次导入来自哪个模板的哪个版本」，
	// 用于导入统计与后续版本更新提示。
	TemplateID      string         `json:"templateId"`
	LatestVersionID string         `json:"latestVersionId"`
	UsageGuide      string         `json:"usageGuide"`
	Project         map[string]any `json:"project"`
}

func ecommerceCanvasWorkflowTemplate() model.CanvasWorkflowTemplate {
	// 示例素材来自生产库真实生成图，压缩后随前端静态资源发布。
	// 模板只引用公开静态路径，不绑定任何用户资源 ID，因此对所有账户都可直接预览。
	imageNode := func(id, title, src string, x, y, w, h float64, meta map[string]any) map[string]any {
		base := map[string]any{
			"content": src, "status": "success", "mimeType": "image/jpeg",
			"naturalWidth": w, "naturalHeight": h, "freeResize": false,
			"workflowSample": true,
		}
		for k, v := range meta {
			base[k] = v
		}
		return map[string]any{
			"id": id, "type": "image", "title": title,
			"position": map[string]any{"x": x, "y": y},
			"width":    w, "height": h, "metadata": base,
		}
	}

	product := imageNode("ecom-product", "① 商品原图（替换为你的商品）", "/workflow-samples/sample-1.jpg",
		80, 120, 380, 380, map[string]any{"workflowRole": "product_input"})

	brief := map[string]any{
		"id": "ecom-brief", "type": "text", "title": "② 商品资料与卖点（可修改）",
		"position": map[string]any{"x": 80.0, "y": 560.0}, "width": 380.0, "height": 320.0,
		"metadata": map[string]any{
			"content":      "商品名称：\n品类：\n材质与规格：\n核心卖点（每行一项）：\n目标人群：\n使用场景：\n视觉风格：高级、干净、真实商业摄影\n\n仅填写已确认的信息，不编造功效、认证、材质或包装内容。",
			"workflowRole": "product_brief",
		},
	}

	mainImage := imageNode("ecom-main", "③ 主图｜商品首图", "/workflow-samples/sample-2.jpg",
		560, 120, 380, 380, map[string]any{
			"workflowRole": "main_image", "generationMode": "image", "generationType": "edit", "size": "1:1",
			"composerContent": "生成 1 张电商平台商品主图。严格保留输入商品的外观、结构、颜色、比例、Logo、包装文字和配件，不改变产品设计。商品为主体，真实商业摄影，光线自然，轮廓清晰，构图适合列表首图，画面干净，不添加未经提供的卖点、认证、折扣、评价、价格、边框或水印。输出仅 1 张独立图片，不拼接。",
			"prompt":          "生成 1 张电商平台商品主图。严格保留输入商品的外观、结构、颜色、比例、Logo、包装文字和配件，不改变产品设计。商品为主体，真实商业摄影，光线自然，轮廓清晰，构图适合列表首图，画面干净，不添加未经提供的卖点、认证、折扣、评价、价格、边框或水印。输出仅 1 张独立图片，不拼接。",
		})

	selling := imageNode("ecom-selling", "④ 详情图｜核心卖点", "/workflow-samples/sample-3.jpg",
		1040, 120, 380, 380, map[string]any{
			"workflowRole": "detail_selling_point", "generationMode": "image", "generationType": "edit", "size": "3:2",
			"composerContent": "生成 1 张独立商品详情图模块，突出商品资料中已经确认的核心卖点。严格保持商品身份一致，用真实细节与使用场景解释卖点，不编造性能或功效。画面预留简洁文案区域，不在图片中生成文字、字母、数字、徽章或水印。输出仅 1 张独立图片，不拼接长图。",
			"prompt":          "生成 1 张独立商品详情图模块，突出商品资料中已经确认的核心卖点。严格保持商品身份一致，用真实细节与使用场景解释卖点，不编造性能或功效。画面预留简洁文案区域，不在图片中生成文字、字母、数字、徽章或水印。输出仅 1 张独立图片，不拼接长图。",
		})

	craft := imageNode("ecom-craft", "⑤ 详情图｜材质与工艺", "/workflow-samples/sample-4.jpg",
		560, 560, 380, 380, map[string]any{
			"workflowRole": "detail_craft", "generationMode": "image", "generationType": "edit", "size": "1:1",
			"composerContent": "生成 1 张独立商品细节图，以真实微距商业摄影展示原图可见的材质、结构、工艺与设计细节。不得虚构不可见结构、材质或配件，不添加文字、参数、认证或水印。保持商品颜色、比例和 Logo 一致。",
			"prompt":          "生成 1 张独立商品细节图，以真实微距商业摄影展示原图可见的材质、结构、工艺与设计细节。不得虚构不可见结构、材质或配件，不添加文字、参数、认证或水印。保持商品颜色、比例和 Logo 一致。",
		})

	scene := imageNode("ecom-scene", "⑥ 详情图｜使用场景", "/workflow-samples/sample-5.jpg",
		1040, 560, 380, 380, map[string]any{
			"workflowRole": "detail_scene", "generationMode": "image", "generationType": "edit", "size": "3:2",
			"composerContent": "生成 1 张独立商品使用场景详情图，依据商品资料中的目标人群和使用场景构建真实生活方式画面。商品仍是视觉主体，外观和比例与输入一致，不虚构使用效果，不添加文字、价格、认证、水印。输出仅 1 张图片。",
			"prompt":          "生成 1 张独立商品使用场景详情图，依据商品资料中的目标人群和使用场景构建真实生活方式画面。商品仍是视觉主体，外观和比例与输入一致，不虚构使用效果，不添加文字、价格、认证、水印。输出仅 1 张图片。",
		})

	packaging := imageNode("ecom-packaging", "⑦ 详情图｜包装与配件", "/workflow-samples/sample-6.jpg",
		560, 1000, 380, 380, map[string]any{
			"workflowRole": "detail_packaging", "generationMode": "image", "generationType": "edit", "size": "3:2",
			"composerContent": "生成 1 张独立商品包装与配件展示图。只展示原图与商品资料中已确认的包装、配件和随附物品，不得虚构未提供的赠品、认证或规格。真实商业摄影，背景干净，不添加文字、价格或水印。",
			"prompt":          "生成 1 张独立商品包装与配件展示图。只展示原图与商品资料中已确认的包装、配件和随附物品，不得虚构未提供的赠品、认证或规格。真实商业摄影，背景干净，不添加文字、价格或水印。",
		})

	closing := imageNode("ecom-closing", "⑧ 详情图｜收尾总结", "/workflow-samples/sample-7.jpg",
		1040, 1000, 380, 380, map[string]any{
			"workflowRole": "detail_closing", "generationMode": "image", "generationType": "edit", "size": "1:1",
			"composerContent": "生成 1 张商品详情页收尾图，用干净构图总结商品整体形象与已确认的核心价值。保持商品外观一致，不新增未提供的卖点、认证、评价或承诺。画面简洁，不添加文字、价格或水印。",
			"prompt":          "生成 1 张商品详情页收尾图，用干净构图总结商品整体形象与已确认的核心价值。保持商品外观一致，不新增未提供的卖点、认证、评价或承诺。画面简洁，不添加文字、价格或水印。",
		})

	nodes := []any{product, brief, mainImage, selling, craft, scene, packaging, closing}

	edges := func(to string) []map[string]any {
		return []map[string]any{
			{"id": "edge-product-" + to, "fromNodeId": "ecom-product", "toNodeId": to},
			{"id": "edge-brief-" + to, "fromNodeId": "ecom-brief", "toNodeId": to},
		}
	}
	connections := []map[string]any{}
	for _, target := range []string{"ecom-main", "ecom-selling", "ecom-craft", "ecom-scene", "ecom-packaging", "ecom-closing"} {
		connections = append(connections, edges(target)...)
	}

	project := map[string]any{
		"id": ecommerceCanvasWorkflowSlug, "title": "电商主图＋详情页套装",
		"nodes": nodes, "connections": connections,
		"chatSessions": []any{}, "activeChatId": nil,
		"backgroundMode": "dark", "showImageInfo": false,
		"viewport": map[string]any{"x": 40.0, "y": 40.0, "k": 0.52},
	}
	payload, _ := json.Marshal(project)
	return model.CanvasWorkflowTemplate{
		ID: "builtin-ecommerce-product-detail-suite-v2", Slug: ecommerceCanvasWorkflowSlug,
		Name:        "电商主图＋详情页套装",
		Description: "8 个节点带真实示例图：商品原图、商品资料、主图、核心卖点、材质工艺、使用场景、包装配件、收尾总结。导入后替换商品图与卖点即可逐张生成。",
		Category:    "电商设计", Version: 2, DraftJSON: string(payload), Published: true, Enabled: true, Builtin: true,
		Revision: 1,
	}
}

// ecommerceVisualSuiteTemplate 是第二个内置模板：视觉套图生产线。
//
// 与「电商主图＋详情页套装」的区别：
//   - 那套是「一个商品 → 主图 + 详情图」的纵向链路；
//   - 这套是「一张参考图 → 多风格视觉变体」的横向扩展，用于投放素材、灵感探索与 A/B 测试。
//
// 布局按参考站真实视觉规范：节点标题在图片上方 22px、图片圆角 10px、
// 连线 1.6px 半透明、点阵背景 22px。
func ecommerceVisualSuiteTemplate() model.CanvasWorkflowTemplate {
	img := func(id, title, src string, x, y, w, h float64, role, prompt string) map[string]any {
		meta := map[string]any{
			"content": src, "status": "success", "mimeType": "image/jpeg",
			"naturalWidth": w, "naturalHeight": h, "freeResize": false,
			"workflowSample": true, "workflowRole": role,
		}
		if prompt != "" {
			meta["composerContent"] = prompt
			meta["prompt"] = prompt
			meta["generationMode"] = "image"
			meta["generationType"] = "edit"
		}
		return map[string]any{
			"id": id, "type": "image", "title": title,
			"position": map[string]any{"x": x, "y": y},
			"width":    w, "height": h, "metadata": meta,
		}
	}

	// 第一列：输入
	source := img("vs-source", "① 参考图（替换为你的商品或模特）", "/workflow-samples-v2/p1.jpg",
		60, 120, 360, 360, "reference_input", "")

	brief := map[string]any{
		"id": "vs-brief", "type": "text", "title": "② 视觉方向（可修改）",
		"position": map[string]any{"x": 60.0, "y": 540.0}, "width": 360.0, "height": 300.0,
		"metadata": map[string]any{
			"content":      "主体：\n品类：\n品牌调性：\n目标人群：\n使用场景：\n\n视觉方向（每行一种，用于横向扩展）：\n1. 极简棚拍\n2. 生活场景\n3. 材质特写\n4. 氛围光影\n\n仅填写已确认的信息，不编造功效、认证或材质。",
			"workflowRole": "visual_brief",
		},
	}

	// 第二列：风格变体（4 个）
	variants := []struct {
		id, title, src, role, prompt string
		x, y, w, h                   float64
	}{
		{"vs-v1", "③ 极简棚拍", "/workflow-samples-v2/p2.jpg", "variant_studio",
			"基于参考图生成 1 张极简棚拍视觉。严格保留主体的外观、结构、颜色、比例、Logo 与包装文字，不改变产品设计。纯色或浅灰背景，柔和均匀布光，主体居中，画面干净留白，不添加文字、价格、认证、水印。",
			520, 60, 360, 360},
		{"vs-v2", "④ 生活场景", "/workflow-samples-v2/p3.jpg", "variant_lifestyle",
			"基于参考图生成 1 张生活方式场景视觉。主体外观与比例保持一致，场景依据视觉方向中的目标人群与使用场景构建。真实自然光，环境细节克制，主体仍是视觉重心，不虚构使用效果，不添加文字或水印。",
			520, 470, 360, 360},
		{"vs-v3", "⑤ 材质特写", "/workflow-samples-v2/p4.jpg", "variant_detail",
			"基于参考图生成 1 张材质与工艺特写。以真实微距商业摄影展示原图可见的材质纹理、结构、工艺与设计细节，不得虚构不可见结构或材质。景深控制得当，不添加文字、参数或认证。",
			520, 880, 360, 360},
		{"vs-v4", "⑥ 氛围光影", "/workflow-samples-v2/p5.jpg", "variant_mood",
			"基于参考图生成 1 张氛围光影视觉。保留主体外观与比例，用有方向性的光线、色彩对比与景深营造情绪，适合投放素材。不改变产品设计，不添加文字、价格或水印。",
			980, 60, 360, 360},
	}

	// 第三列：投放与延展
	deliver := []struct {
		id, title, src, role, prompt string
		x, y, w, h                   float64
	}{
		{"vs-d1", "⑦ 竖版投放（3:4）", "/workflow-samples-v2/p7.jpg", "deliver_vertical",
			"基于参考图生成 1 张 3:4 竖版投放素材。主体完整可见，上下预留安全区避免平台 UI 遮挡，构图适合信息流。保持产品外观一致，不添加文字、价格、水印。",
			980, 470, 360, 480},
		{"vs-d2", "⑧ 横版投放（16:9）", "/workflow-samples-v2/p9.jpg", "deliver_horizontal",
			"基于参考图生成 1 张 16:9 横版投放素材。主体位于视觉重心，左右留出文案安全区，构图适合横幅与开屏。保持产品外观一致，不添加文字、价格、水印。",
			980, 1000, 480, 270},
		{"vs-d3", "⑨ 细节延展", "/workflow-samples-v2/p6.jpg", "deliver_detail",
			"基于参考图生成 1 张细节延展视觉，突出一个原图可见的关键结构或质感。保持主体身份一致，不虚构配件，不添加文字或认证。",
			520, 1290, 360, 360},
		{"vs-d4", "⑩ 收尾总结", "/workflow-samples-v2/p10.jpg", "deliver_closing",
			"基于参考图生成 1 张收尾视觉，用干净构图总结主体整体形象。保持外观一致，不新增未提供的卖点、认证或承诺，不添加文字、价格、水印。",
			980, 1330, 360, 480},
	}

	nodes := []any{source, brief}
	for _, v := range variants {
		nodes = append(nodes, img(v.id, v.title, v.src, v.x, v.y, v.w, v.h, v.role, v.prompt))
	}
	for _, d := range deliver {
		nodes = append(nodes, img(d.id, d.title, d.src, d.x, d.y, d.w, d.h, d.role, d.prompt))
	}

	connections := []map[string]any{}
	for _, target := range []string{"vs-v1", "vs-v2", "vs-v3", "vs-v4", "vs-d1", "vs-d2", "vs-d3", "vs-d4"} {
		connections = append(connections,
			map[string]any{"id": "edge-source-" + target, "fromNodeId": "vs-source", "toNodeId": target},
			map[string]any{"id": "edge-brief-" + target, "fromNodeId": "vs-brief", "toNodeId": target},
		)
	}

	project := map[string]any{
		"id": "ecommerce-visual-suite", "title": "视觉套图生产线",
		"nodes": nodes, "connections": connections,
		"chatSessions": []any{}, "activeChatId": nil,
		"backgroundMode": "dark", "showImageInfo": false,
		"viewport": map[string]any{"x": 30.0, "y": 30.0, "k": 0.46},
	}
	payload, _ := json.Marshal(project)
	return model.CanvasWorkflowTemplate{
		ID: "builtin-ecommerce-visual-suite-v1", Slug: "ecommerce-visual-suite",
		Name:        "视觉套图生产线",
		Description: "一张参考图横向扩展成 8 组视觉：极简棚拍、生活场景、材质特写、氛围光影，外加竖版/横版投放、细节延展与收尾。适合投放素材与 A/B 测试。",
		Category:    "电商设计", Version: 1, DraftJSON: string(payload),
		Published: true, Enabled: true, Builtin: true, Revision: 1,
	}
}

// ecommerceDressShowcaseTemplate builds a graph matching the supplied 211476 screenshot.
func ecommerceDressShowcaseTemplate() model.CanvasWorkflowTemplate {
	image := func(id, title, file, role, prompt string, x, y, w, h float64) map[string]any {
		meta := map[string]any{"content": "/workflow-samples-reference-211476/" + file, "status": "success", "mimeType": "image/jpeg", "workflowSample": true, "workflowRole": role, "naturalWidth": w, "naturalHeight": h}
		if prompt != "" {
			meta["prompt"] = prompt
			meta["composerContent"] = prompt
			meta["generationMode"] = "image"
			meta["generationType"] = "edit"
			meta["size"] = "3:4"
		}
		return map[string]any{"id": id, "type": "image", "title": title, "position": map[string]any{"x": x, "y": y}, "width": w, "height": h, "metadata": meta}
	}
	text := func(id, title, content string, x, y, w, h float64) map[string]any {
		return map[string]any{"id": id, "type": "text", "title": title, "position": map[string]any{"x": x, "y": y}, "width": w, "height": h, "metadata": map[string]any{"content": content, "workflowRole": "instruction"}}
	}
	productPrompt := "女装电商模特图。严格保持参考模特身份、脸型、发型、身材比例；复现参考服装：黑色无袖上衣、黑白植物印花长裙、黑色平底鞋。摄影棚浅灰墙面与浅棕地面，柔和自然光，真实商业摄影，无文字、水印或额外道具。"

	// 左列：三张原始输入素材。
	inputs := []any{
		image("dress-input-seated", "模特场景参考｜image.png", "seated-source.jpg", "input_scene", "", 40, 300, 170, 255),
		image("dress-input-portrait", "模特肖像参考", "portrait.jpg", "input_portrait", "", 40, 40, 170, 213),
		image("dress-input-flatlay", "服装平铺与鞋履", "flatlay.jpg", "input_product", "", 40, 580, 170, 255),
	}

	// 中间左列：三视图和背景底板。
	middle := []any{
		image("dress-output-three-view", "三视图｜正面 / 侧面 / 背面", "three-view.jpg", "intermediate_three_view", "根据人物肖像和服装平铺参考，生成同一模特穿着该套服装的正面、侧面、背面三视图；身份、发型、服装纹样保持一致，背景统一。", 320, 250, 200, 267),
		image("dress-output-floor", "场景底板提取", "scene-floor.jpg", "intermediate_scene", "提取参考场景中的浅灰墙面与浅棕地面，保留交界线、透视和光线，移除人物及服装。", 320, 500, 200, 301),
	}

	// 中上：两块文字规划节点。
	analysis := text("dress-analysis", "素材类别：服装展示｜素材分析", "素材类别：服装展示\n\n素材分析：\n- Image1：人物参考，锁定脸型、发型、身材，生成正侧背。\n- Image2：人物肖像与妆容参考，保持人物身份一致。\n- Image3：服装平铺，锁定黑色上衣、印花长裙和鞋履。\n\n全部输出使用同一人物和同一套服装。", 520, 35, 220, 190)
	production := text("dress-production", "动作与镜头提示词", "动作1：正面全身展示，模特自然站立，服装完整清晰。\n\n动作2：同一模特侧面自然行走，裙摆轻微摆动。\n\n动作3：背面全身展示，保持服装结构与图案一致。\n\n动作4：模特坐在白色椅子上，姿态自然。\n\n统一浅灰墙面、浅棕地面、柔和自然光；真实目录摄影，无文字、无水印。", 760, 280, 310, 240)

	// 中右竖列：五张不同动作/视角的输出结果。
	outputs := []any{
		image("dress-front", "模特成片｜正面全身", "front.jpg", "output_front", productPrompt, 990, 45, 190, 255),
		image("dress-walk", "模特成片｜侧面行走", "walk.jpg", "output_walk", "同一模特侧面行走，保持五官、发型、服装图案和鞋履一致。", 990, 335, 190, 255),
		image("dress-back", "模特成片｜背面展示", "back.jpg", "output_back", "同一模特背对镜头站立，展示裙装背面与整体版型。", 990, 625, 190, 255),
		image("dress-sit", "模特成片｜白椅坐姿", "sitting.jpg", "output_sitting", "同一模特坐在简洁白色椅子上，自然姿势展示整套服装。", 990, 915, 190, 255),
		image("dress-side", "模特成片｜侧身站姿", "side.jpg", "output_side", "同一模特侧身自然站立，清晰展示裙装轮廓和印花。", 990, 1205, 190, 255),
	}

	// 最右列：两张正面版本成片。
	variants := []any{
		image("dress-front-a", "模特成片｜版本 A", "front.jpg", "output_front_variant_a", "正面全身商业目录照，模特自然站姿，保持身份、服饰和背景一致。", 1290, 45, 190, 255),
		image("dress-front-b", "模特成片｜版本 B", "front.jpg", "output_front_variant_b", "正面全身商业目录照，模特正对镜头，姿态简洁，保持人物和服装一致。", 1590, 45, 190, 255),
	}

	nodes := make([]any, 0, 14)
	nodes = append(nodes, inputs...)
	nodes = append(nodes, middle...)
	nodes = append(nodes, analysis, production)
	nodes = append(nodes, outputs...)
	nodes = append(nodes, variants...)

	connections := []map[string]any{}
	addEdge := func(from, to string) {
		connections = append(connections, map[string]any{"id": fmt.Sprintf("dress-edge-%02d", len(connections)+1), "fromNodeId": from, "toNodeId": to})
	}
	for _, input := range []string{"dress-input-seated", "dress-input-portrait", "dress-input-flatlay"} {
		for _, mid := range []string{"dress-output-three-view", "dress-output-floor", "dress-analysis", "dress-production"} {
			addEdge(input, mid)
		}
	}
	for _, output := range []string{"dress-front", "dress-walk", "dress-back", "dress-sit", "dress-side", "dress-front-a", "dress-front-b"} {
		for _, mid := range []string{"dress-output-three-view", "dress-output-floor", "dress-analysis", "dress-production"} {
			addEdge(mid, output)
		}
	}

	project := map[string]any{
		"id":    "ecommerce-dress-showcase-2114764788576673794",
		"title": "电商女装模特展示｜14 节点",
		"nodes": nodes, "connections": connections,
		"chatSessions": []any{}, "activeChatId": nil,
		"backgroundMode": "light", "showImageInfo": false,
		"viewport": map[string]any{"x": -10.0, "y": 0.0, "k": 0.55},
	}
	payload, _ := json.Marshal(project)
	return model.CanvasWorkflowTemplate{
		ID:          "builtin-ecommerce-dress-showcase-211476-v1",
		Slug:        "ecommerce-dress-showcase-211476",
		Name:        "电商女装模特展示｜14 节点复刻",
		Description: "按参考截图复刻：3 张输入素材、三视图与场景底板、2 个说明节点、5 张竖排模特结果及 2 张正面成片。素材来自用户提供的画布素材包。",
		Category:    "电商设计", Version: 1, DraftJSON: string(payload),
		Published: true, Enabled: true, Builtin: true, Revision: 1,
	}
}

func (s *Service) ensureBuiltinCanvasWorkflowTemplates() error {
	items := []model.CanvasWorkflowTemplate{
		ecommerceCanvasWorkflowTemplate(),
		ecommerceVisualSuiteTemplate(),
		ecommerceDressShowcaseTemplate(),
	}
	embedded, err := loadEmbeddedCanvasWorkflowTemplates()
	if err != nil {
		return err
	}
	items = append(items, embedded...)
	for _, item := range items {
		if err := s.ensureOneBuiltinCanvasWorkflow(item); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureOneBuiltinCanvasWorkflow(item model.CanvasWorkflowTemplate) error {
	if item.DraftDescription == "" {
		item.DraftDescription = item.Description
	}
	existing, err := s.repo.CanvasWorkflowTemplateBySlug(item.Slug)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// 首次启动或生产库还原后没有这条记录：写入主体并发布 v1。
		if err := s.repo.SaveCanvasWorkflowTemplate(&item); err != nil {
			return err
		}
		_, err := s.publishCanvasWorkflow(item.ID, "内置模板初始版本", "")
		return err
	}
	// 内置模板只在「从未被管理员改动过」时跟随代码升级。
	// 一旦管理员在后台编辑过（revision > 1），草稿就是运营真相，
	// 代码里的默认内容不得再覆盖它，否则每次保存都会被回滚。
	if existing.Revision > 1 {
		return nil
	}
	if existing.DraftJSON == item.DraftJSON {
		return nil
	}
	// 首次升级：沿用原记录 ID，刷新草稿并发布新版本，保留历史快照。
	item.ID = existing.ID
	item.Revision = existing.Revision + 1
	item.LatestVersionID = existing.LatestVersionID
	item.Version = existing.Version
	if err := s.repo.SaveCanvasWorkflowTemplate(&item); err != nil {
		return err
	}
	_, err = s.publishCanvasWorkflow(item.ID, "内置模板升级", "")
	return err
}

func (s *Service) CanvasWorkflowTemplates(userID string) ([]CanvasWorkflowTemplateSummary, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, BadAuthRequest("请先登录")
	}
	if err := s.ensureBuiltinCanvasWorkflowTemplates(); err != nil {
		return nil, err
	}
	templates, err := s.repo.VisibleCanvasWorkflowTemplates()
	if err != nil {
		return nil, err
	}
	result := make([]CanvasWorkflowTemplateSummary, 0, len(templates))
	ids := make([]string, 0, len(templates))
	for _, item := range templates {
		ids = append(ids, item.ID)
	}
	stats, err := s.repo.CanvasWorkflowImportStats(ids)
	if err != nil {
		return nil, err
	}
	for _, item := range templates {
		summary, err := s.canvasWorkflowSummary(item)
		if err != nil {
			// 单个模板数据损坏时跳过，不让整个列表 500。
			continue
		}
		summary.ImportTotal = stats[item.ID].Total
		result = append(result, summary)
	}
	return result, nil
}

func (s *Service) CanvasWorkflowTemplate(userID, slug string) (CanvasWorkflowTemplateDetail, error) {
	if strings.TrimSpace(userID) == "" {
		return CanvasWorkflowTemplateDetail{}, BadAuthRequest("请先登录")
	}
	if err := s.ensureBuiltinCanvasWorkflowTemplates(); err != nil {
		return CanvasWorkflowTemplateDetail{}, err
	}
	item, err := s.repo.VisibleCanvasWorkflowTemplateBySlug(slug)
	if err != nil {
		return CanvasWorkflowTemplateDetail{}, err
	}
	payload, version, err := s.canvasWorkflowPublishedPayload(*item)
	if err != nil {
		return CanvasWorkflowTemplateDetail{}, err
	}
	summary, err := s.canvasWorkflowSummary(*item)
	if err != nil {
		return CanvasWorkflowTemplateDetail{}, err
	}
	summary.Version = version
	versionSnapshot, err := s.repo.CanvasWorkflowVersion(item.LatestVersionID)
	if err != nil {
		return CanvasWorkflowTemplateDetail{}, err
	}
	var project map[string]any
	if err := json.Unmarshal([]byte(payload), &project); err != nil {
		return CanvasWorkflowTemplateDetail{}, fmt.Errorf("工作流模板格式无效: %w", err)
	}
	return CanvasWorkflowTemplateDetail{CanvasWorkflowTemplateSummary: summary, TemplateID: item.ID, LatestVersionID: item.LatestVersionID, UsageGuide: versionSnapshot.UsageGuide, Project: project}, nil
}

// canvasWorkflowPublishedPayload 返回前台唯一可信内容：LatestVersionID 指向的只读快照。
// 草稿永远不对外暴露，避免管理员编辑中的半成品被用户看到。
func (s *Service) canvasWorkflowPublishedPayload(item model.CanvasWorkflowTemplate) (string, int, error) {
	if strings.TrimSpace(item.LatestVersionID) == "" {
		return "", 0, BadAuthRequest("该工作流尚未发布")
	}
	version, err := s.repo.CanvasWorkflowVersion(item.LatestVersionID)
	if err != nil {
		return "", 0, err
	}
	return version.PayloadJSON, version.Version, nil
}

func (s *Service) canvasWorkflowSummary(item model.CanvasWorkflowTemplate) (CanvasWorkflowTemplateSummary, error) {
	payload, version, err := s.canvasWorkflowPublishedPayload(item)
	if err != nil {
		return CanvasWorkflowTemplateSummary{}, err
	}
	nodeCount := canvasWorkflowNodeCount(payload)
	if nodeCount == 0 {
		return CanvasWorkflowTemplateSummary{}, fmt.Errorf("工作流模板 %s 没有有效节点", item.Slug)
	}
	versionSnapshot, err := s.repo.CanvasWorkflowVersion(item.LatestVersionID)
	if err != nil {
		return CanvasWorkflowTemplateSummary{}, err
	}
	description := versionSnapshot.Description
	if description == "" {
		description = item.Description
	}
	coverURL := ""
	if versionSnapshot.CoverResourceID != "" {
		coverURL = "/api/canvas-workflows/" + url.PathEscape(item.Slug) + "/cover"
	}
	return CanvasWorkflowTemplateSummary{
		Slug: item.Slug, Name: item.Name, Description: description,
		Category: item.Category, Version: version, Builtin: item.Builtin, NodeCount: nodeCount,
		MediaTypes: canvasWorkflowMediaTypes(payload), CoverURL: coverURL, UpdatedAt: versionSnapshot.CreatedAt,
	}, nil
}

func canvasWorkflowMediaTypes(payload string) []string {
	var document struct {
		Nodes []map[string]any `json:"nodes"`
	}
	if json.Unmarshal([]byte(payload), &document) != nil {
		return nil
	}
	set := map[string]bool{}
	for _, node := range document.Nodes {
		typeName := strings.ToLower(strings.TrimSpace(kernel.StringValue(node["type"])))
		metadata, _ := node["metadata"].(map[string]any)
		mode := strings.ToLower(kernel.StringValue(metadata["generationMode"]))
		if mode == "image" || mode == "video" || mode == "audio" || mode == "text" {
			set[mode] = true
		}
		switch typeName {
		case "image", "video", "audio", "text":
			set[typeName] = true
		case "caption", "script", "markdown":
			set["text"] = true
		case "result":
			source, _ := metadata["workflowSource"].(map[string]any)
			taskType := strings.ToLower(kernel.StringValue(metadata["taskType"]))
			if taskType == "" {
				taskType = strings.ToLower(kernel.StringValue(source["taskType"]))
			}
			if strings.Contains(taskType, "video") {
				set["video"] = true
			} else {
				set["image"] = true
			}
		}
	}
	result := make([]string, 0, len(set))
	for mediaType := range set {
		result = append(result, mediaType)
	}
	sort.Strings(result)
	return result
}

func (s *Service) ImportCanvasWorkflowTemplate(userID, slug string) (*model.CanvasProject, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, BadAuthRequest("请先登录")
	}
	if _, err := s.repo.User(userID); err != nil {
		return nil, BadAuthRequest("账户不可用，请重新登录")
	}
	detail, err := s.CanvasWorkflowTemplate(userID, slug)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	canvasID := kernel.NewID()
	project, err := remapCanvasWorkflowProject(detail.Project, detail.Slug, detail.Version)
	if err != nil {
		return nil, err
	}
	project["id"] = canvasID
	project["title"] = detail.Name
	project["createdAt"] = now
	project["updatedAt"] = now
	project["chatSessions"] = []any{}
	project["activeChatId"] = nil
	payload, err := json.Marshal(project)
	if err != nil {
		return nil, err
	}
	imported := &model.CanvasProject{ID: canvasID, UserID: userID, Title: detail.Name, PayloadJSON: string(payload), Revision: 0, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.UpsertCanvasProject(imported); err != nil {
		return nil, err
	}
	// 记录导入来源版本：既用于运营统计，也用于后续「模板已更新」提示。
	// 统计失败不能影响用户已经成功的导入，因此只记日志。
	if err := s.repo.SaveCanvasWorkflowImport(&model.CanvasWorkflowImport{
		ID:         kernel.NewID(),
		TemplateID: detail.TemplateID,
		VersionID:  detail.LatestVersionID,
		Version:    detail.Version,
		UserID:     userID,
		CanvasID:   canvasID,
		CreatedAt:  now,
	}); err != nil {
		// 不影响主流程
		_ = err
	}
	return imported, nil
}

// publishCanvasWorkflow 把当前草稿固化成新的只读版本，并让前台指向它。
// 草稿内容不会被清空：管理员可以继续在同一份草稿上迭代下一版。
func (s *Service) publishCanvasWorkflow(templateID, name, notes string) (*model.CanvasWorkflowVersion, error) {
	item, err := s.repo.CanvasWorkflowTemplateByID(templateID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(item.DraftJSON) == "" {
		return nil, BadAuthRequest("草稿为空，无法发布")
	}
	if err := validateCanvasWorkflowPayload(item.DraftJSON); err != nil {
		return nil, err
	}
	next := item.Version + 1
	description := strings.TrimSpace(item.DraftDescription)
	if description == "" {
		description = item.Description
	}
	version := &model.CanvasWorkflowVersion{
		ID:              kernel.NewID(),
		TemplateID:      item.ID,
		Version:         next,
		Name:            strings.TrimSpace(name),
		Notes:           strings.TrimSpace(notes),
		Description:     description,
		UsageGuide:      strings.TrimSpace(item.DraftUsageGuide),
		CoverResourceID: strings.TrimSpace(item.DraftCoverResourceID),
		PayloadJSON:     item.DraftJSON,
		NodeCount:       canvasWorkflowNodeCount(item.DraftJSON),
		CreatedAt:       time.Now(),
	}
	if err := s.repo.PublishCanvasWorkflowVersion(version, item.Revision); err != nil {
		return nil, err
	}
	return version, nil
}

// validateCanvasWorkflowPayload 是模板入库边界：体积、节点数、内嵌媒体都要拦。
// 模板会被每个用户下载一次，必须比普通画布更严格。
func validateCanvasWorkflowPayload(payload string) error {
	if len(payload) > 1<<20 {
		return BadAuthRequest("模板数据超过 1MB，请精简节点或改用资源引用")
	}
	var document struct {
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		return BadAuthRequest("模板数据不是合法 JSON")
	}
	if len(document.Nodes) == 0 {
		return BadAuthRequest("模板至少需要一个节点")
	}
	if len(document.Nodes) > 200 {
		return BadAuthRequest("模板节点数不能超过 200 个")
	}
	nodeIDs := make(map[string]struct{}, len(document.Nodes))
	for _, node := range document.Nodes {
		nodeID := kernel.StringValue(node["id"])
		if nodeID == "" {
			return BadAuthRequest("模板节点缺少 id")
		}
		if _, exists := nodeIDs[nodeID]; exists {
			return BadAuthRequest("模板节点 id 重复: " + nodeID)
		}
		nodeIDs[nodeID] = struct{}{}
		metadata, _ := node["metadata"].(map[string]any)
		for key := range metadata {
			if isCanvasWorkflowSensitiveKey(key) {
				return BadAuthRequest("模板节点包含运行时字段 " + key + "，请先清理")
			}
		}
		for _, key := range []string{"taskId", "generationTaskId", "taskStatus", "taskProgress", "generationBatches", "agentDraftRunId", "errorDetails", "apiKey", "storageKey"} {
			if _, exists := metadata[key]; exists {
				return BadAuthRequest("模板节点包含运行时字段 " + key + "，请先清理")
			}
		}
		if content, ok := metadata["content"].(string); ok && strings.HasPrefix(content, "/api/resources/") {
			return BadAuthRequest("模板节点引用了私有资源，请改用公开静态路径")
		}
	}
	return nil
}

// canvasWorkflowNodeCount 统计模板节点数；解析失败返回 0，由调用方决定是否报错。
func canvasWorkflowNodeCount(payload string) int {
	var document struct {
		Nodes []any `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		return 0
	}
	return len(document.Nodes)
}
