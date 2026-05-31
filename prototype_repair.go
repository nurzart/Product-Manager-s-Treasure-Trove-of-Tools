package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const prototypeRenderModeProjectRepair = "project_repair"

func parseLooseJSONObject(payload string) map[string]any {
	raw := map[string]any{}
	if strings.TrimSpace(payload) == "" {
		return raw
	}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return map[string]any{}
	}
	return raw
}

func cachedPrototypeRenderBundleSpec(generationMeta string) (PrototypeRenderBundleSpec, bool) {
	raw := parseLooseJSONObject(generationMeta)
	value, ok := raw["prototype_render_bundle"]
	if !ok {
		return PrototypeRenderBundleSpec{}, false
	}
	spec, err := parsePrototypeRenderBundleJSON(mustJSON(value))
	if err != nil {
		return PrototypeRenderBundleSpec{}, false
	}
	return normalizePrototypeRenderBundleSpec(spec), true
}

func cachedPrototypeRepairReport(generationMeta string) (PrototypeRepairReport, bool) {
	raw := parseLooseJSONObject(generationMeta)
	value, ok := raw["prototype_repair_report"]
	if !ok {
		return PrototypeRepairReport{}, false
	}
	var report PrototypeRepairReport
	if err := json.Unmarshal([]byte(mustJSON(value)), &report); err != nil {
		return PrototypeRepairReport{}, false
	}
	report.Diagnostics = ensureSlice(report.Diagnostics)
	return report, true
}

func mergePrototypeGenerationMeta(existing string, spec PrototypeRenderBundleSpec, report PrototypeRepairReport, diagnostics []PrototypeDiagnostic) string {
	raw := parseLooseJSONObject(existing)
	raw["prototype_render_bundle"] = normalizePrototypeRenderBundleSpec(spec)
	raw["prototype_repair_report"] = report
	raw["prototype_diagnostics"] = ensureSlice(diagnostics)
	raw["prototype_last_repaired_at"] = nowUTC().Format(timeLayout)
	return mustJSON(raw)
}

func diagnosePrototypeRenderSpec(schema UISchema, spec PrototypeRenderBundleSpec) []PrototypeDiagnostic {
	html := strings.ToLower(strings.TrimSpace(spec.PreviewHTML))
	diagnostics := []PrototypeDiagnostic{}

	if len(schema.Routes) > 1 {
		if !strings.Contains(html, "<script") && !strings.Contains(html, "onclick=") {
			diagnostics = append(diagnostics, PrototypeDiagnostic{
				Code:     "navigation-static",
				Severity: "warning",
				Summary:  "检测到这是静态视觉稿，导航很可能无法切换页面。",
				Detail:   "当前预览包含多个页面路由，但没有发现可执行脚本或点击事件绑定。",
			})
		}
		if !strings.Contains(html, "<button") && !strings.Contains(html, "<a ") && !strings.Contains(html, "role=\"button\"") {
			diagnostics = append(diagnostics, PrototypeDiagnostic{
				Code:     "navigation-no-interactive-elements",
				Severity: "warning",
				Summary:  "导航区域缺少明显的可点击元素。",
				Detail:   "当前 HTML 中没有检测到 button、a 或 button role，菜单大概率只是静态文本。",
			})
		}
	}

	if strings.TrimSpace(spec.AppTSX) == "" || strings.TrimSpace(spec.PreviewHTML) == "" {
		diagnostics = append(diagnostics, PrototypeDiagnostic{
			Code:     "bundle-incomplete",
			Severity: "warning",
			Summary:  "当前原型 bundle 不完整。",
			Detail:   "缺少可用的页面代码或预览 HTML，预览与导出结果可能不稳定。",
		})
	}

	if spec.RenderMode == prototypeRenderModeFallback {
		diagnostics = append(diagnostics, PrototypeDiagnostic{
			Code:     "fallback-renderer",
			Severity: "info",
			Summary:  "当前正在展示内置兜底原型。",
			Detail:   "这不是 AI 原始页面，而是系统根据 UISchema 渲染的安全兜底视图。",
		})
	}

	return ensureSlice(diagnostics)
}

func buildPrototypeRepairPrompt(project Project, schema UISchema, currentSpec PrototypeRenderBundleSpec, input PrototypeRepairInput, diagnostics []PrototypeDiagnostic, generationMeta string) string {
	contextPayload := map[string]any{
		"project_name":          project.Name,
		"project_description":   project.Description,
		"ui_schema":             schema,
		"generation_meta":       generationMeta,
		"current_render_bundle": currentSpec,
		"diagnostics":           diagnostics,
		"repair_request": map[string]any{
			"page_id":            strings.TrimSpace(input.PageID),
			"page_title":         findUIPageTitle(schema, input.PageID),
			"current_issue":      strings.TrimSpace(input.CurrentIssue),
			"expected_behavior":  strings.TrimSpace(input.ExpectedBehavior),
			"optimization_notes": strings.TrimSpace(input.OptimizationNotes),
		},
	}
	return strings.Join([]string{
		"请基于以下 CONTEXT_JSON，为当前项目输出一个 prototype_render_bundle JSON 对象，用于修复原型交互问题。",
		"你不是在重做一个通用后台模板，而是在修复当前项目自己的原始页面。",
		"输出字段必须只包含：render_mode, design_summary, app_tsx, styles_css, preview_html。",
		"必须遵守：",
		"1. 保留当前项目的信息架构、页面命名和主要视觉方向，不要改造成另一套通用管理台。",
		"2. 必须修复 repair_request 中提到的问题，尤其是导航、菜单、表单、页面切换等交互。",
		"3. preview_html 必须是完整 HTML，且包含可运行交互，不依赖外链资源。",
		"4. app_tsx 必须与 preview_html 保持一致，能表达相同的导航与页面切换逻辑。",
		"5. 如果 repair_request 指定了 page_id，优先修复该页面及其相关导航，不要只输出静态视觉稿。",
		"6. 所有字段值优先使用中文。",
		"7. render_mode 固定输出 project_repair。",
		mustJSON(contextPayload),
	}, "\n")
}

func localPrototypeRepairBundle(project Project, schema UISchema, currentSpec PrototypeRenderBundleSpec, input PrototypeRepairInput) PrototypeRenderBundleSpec {
	issue := strings.TrimSpace(input.CurrentIssue)
	if issue == "" {
		issue = "未提供具体问题，已按当前项目结构重新生成可交互预览。"
	}
	styles := strings.TrimSpace(currentSpec.StylesCSS)
	if styles == "" {
		styles = prototypeStylesCSS()
	}
	return PrototypeRenderBundleSpec{
		RenderMode:    prototypeRenderModeProjectRepair,
		DesignSummary: fmt.Sprintf("未拿到可用 AI 修复结果，已根据当前项目的 UISchema 对原型进行项目级修复。当前重点问题：%s", issue),
		AppTSX:        prototypeAppTSX(),
		StylesCSS:     styles,
		PreviewHTML:   prototypePreviewHTML(project, schema),
	}
}

func buildPrototypeRepairReport(input PrototypeRepairInput, strategy string, diagnostics []PrototypeDiagnostic) PrototypeRepairReport {
	summary := "已完成当前原型修复。"
	if strings.TrimSpace(input.PageID) != "" {
		summary = fmt.Sprintf("已完成页面 %s 的原型修复。", strings.TrimSpace(input.PageID))
	}
	return PrototypeRepairReport{
		Applied:           true,
		Strategy:          strategy,
		Summary:           summary,
		Issue:             strings.TrimSpace(input.CurrentIssue),
		ExpectedBehavior:  strings.TrimSpace(input.ExpectedBehavior),
		OptimizationNotes: strings.TrimSpace(input.OptimizationNotes),
		TargetPageID:      strings.TrimSpace(input.PageID),
		Diagnostics:       ensureSlice(diagnostics),
	}
}

func findUIPageTitle(schema UISchema, pageID string) string {
	pageID = strings.TrimSpace(pageID)
	for _, page := range schema.Pages {
		if page.ID == pageID {
			return page.Title
		}
	}
	return ""
}
