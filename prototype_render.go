package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
)

const (
	prototypeRenderModeAIOriginal = "ai_original"
	prototypeRenderModeFallback   = "fallback_template"
)

type PrototypeRenderBundleSpec struct {
	RenderMode    string `json:"render_mode,omitempty"`
	DesignSummary string `json:"design_summary,omitempty"`
	AppTSX        string `json:"app_tsx,omitempty"`
	StylesCSS     string `json:"styles_css,omitempty"`
	PreviewHTML   string `json:"preview_html,omitempty"`
}

func parsePrototypeRenderBundleJSON(payload string) (PrototypeRenderBundleSpec, error) {
	var spec PrototypeRenderBundleSpec
	if strings.TrimSpace(payload) == "" {
		return spec, errors.New("empty prototype render bundle payload")
	}
	if err := json.Unmarshal([]byte(payload), &spec); err == nil {
		spec = normalizePrototypeRenderBundleSpec(spec)
		if spec.PreviewHTML != "" || spec.AppTSX != "" {
			return spec, nil
		}
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return spec, err
	}
	spec = normalizePrototypeRenderBundleSpec(PrototypeRenderBundleSpec{
		RenderMode:    readStringAny(raw["render_mode"], readStringAny(raw["renderMode"], prototypeRenderModeAIOriginal)),
		DesignSummary: readStringAny(raw["design_summary"], readStringAny(raw["designSummary"], "")),
		AppTSX:        readStringAny(raw["app_tsx"], readStringAny(raw["appTsx"], "")),
		StylesCSS:     readStringAny(raw["styles_css"], readStringAny(raw["stylesCss"], "")),
		PreviewHTML:   readStringAny(raw["preview_html"], readStringAny(raw["previewHtml"], "")),
	})
	if spec.PreviewHTML == "" && spec.AppTSX == "" {
		return PrototypeRenderBundleSpec{}, errors.New("prototype render bundle did not include preview_html or app_tsx")
	}
	return spec, nil
}

func normalizePrototypeRenderBundleSpec(spec PrototypeRenderBundleSpec) PrototypeRenderBundleSpec {
	spec.RenderMode = strings.TrimSpace(spec.RenderMode)
	if spec.RenderMode == "" {
		spec.RenderMode = prototypeRenderModeAIOriginal
	}
	spec.DesignSummary = strings.TrimSpace(spec.DesignSummary)
	spec.AppTSX = strings.TrimSpace(spec.AppTSX)
	spec.StylesCSS = strings.TrimSpace(spec.StylesCSS)
	spec.PreviewHTML = strings.TrimSpace(spec.PreviewHTML)
	return spec
}

func buildPrototypeRenderPrompt(project Project, schema UISchema, generationMeta string) string {
	contextPayload := map[string]any{
		"project_name":        project.Name,
		"project_description": project.Description,
		"ui_schema":           schema,
		"generation_meta":     generationMeta,
	}
	return strings.Join([]string{
		"请基于以下 CONTEXT_JSON，为当前产品原型输出一个 prototype_render_bundle JSON 对象。",
		"目标不是输出结构化骨架，而是输出 AI 设计的原始页面代码与原始页面预览。",
		"输出字段必须只包含：render_mode, design_summary, app_tsx, styles_css, preview_html。",
		"约束如下：",
		"1. app_tsx 必须是完整可运行的 React 组件代码，默认导出 App，且只依赖 React 与 ./schema。",
		"2. styles_css 必须是一份完整样式文件，禁止依赖外部 CSS 框架。",
		"3. preview_html 必须是一份完整 HTML 文档，可直接在 iframe srcdoc 中运行，不依赖 CDN、外链脚本或外链字体。",
		"4. 页面导航、布局、卡片、表单、表格和详情区要尽量体现产品语义，不要套用通用占位稿。",
		"5. 每个页面都要体现真实业务内容，尽量使用 ui_schema 中的 routes/pages/mock_data/actions。",
		"6. 所有字段值优先使用中文，视觉风格要像真实可演示页面。",
		"7. render_mode 固定输出 ai_original。",
		mustJSON(contextPayload),
	}, "\n")
}

func buildFallbackPrototypeRenderBundle(project Project, schema UISchema) PrototypeRenderBundleSpec {
	return PrototypeRenderBundleSpec{
		RenderMode:    prototypeRenderModeFallback,
		DesignSummary: "当前回退到内置原型模板渲染，因为未拿到可用的 AI 原始页面代码。",
		AppTSX:        prototypeAppTSX(),
		StylesCSS:     prototypeStylesCSS(),
		PreviewHTML:   prototypePreviewHTML(project, schema),
	}
}

func prototypePreviewHTML(project Project, schema UISchema) string {
	navButtons := make([]string, 0, len(schema.Routes))
	sections := make([]string, 0, len(schema.Pages))
	for index, route := range schema.Routes {
		page := findUIPageByID(schema.Pages, route.PageID)
		activeClass := ""
		hiddenAttr := " hidden"
		if index == 0 {
			activeClass = " active"
			hiddenAttr = ""
		}
		navButtons = append(navButtons, fmt.Sprintf(`<button class="nav-item%s" data-target="%s">%s</button>`, activeClass, html.EscapeString(page.ID), html.EscapeString(route.Label)))
		sections = append(sections, buildPreviewSectionHTML(schema, page, hiddenAttr))
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>%s</title>
    <style>%s</style>
  </head>
  <body>
    <div class="preview-shell">
      <aside class="preview-sidebar">
        <p class="preview-eyebrow">AI Original Preview</p>
        <h1>%s</h1>
        <p class="preview-description">%s</p>
        <div class="preview-nav">%s</div>
      </aside>
      <main class="preview-main">%s</main>
    </div>
    <script>
      const navItems = Array.from(document.querySelectorAll('.nav-item'));
      const pages = Array.from(document.querySelectorAll('.page-section'));
      function activate(target) {
        navItems.forEach((item) => item.classList.toggle('active', item.dataset.target === target));
        pages.forEach((page) => {
          page.hidden = page.dataset.pageId !== target;
        });
      }
      navItems.forEach((item) => item.addEventListener('click', () => activate(item.dataset.target)));
    </script>
  </body>
</html>`,
		html.EscapeString(project.Name+" 原型预览"),
		prototypePreviewCSS(),
		html.EscapeString(schema.AppMeta.Name),
		html.EscapeString(schema.AppMeta.Description),
		strings.Join(navButtons, ""),
		strings.Join(sections, ""),
	)
}

func buildPreviewSectionHTML(schema UISchema, page UIPage, hiddenAttr string) string {
	pageType := strings.ToLower(strings.TrimSpace(page.Type))
	if pageType == "" {
		pageType = "detail"
	}

	switch pageType {
	case "dashboard":
		return fmt.Sprintf(`<section class="page-section" data-page-id="%s"%s><div class="page-card"><p class="page-eyebrow">%s</p><h2>%s</h2><p class="page-desc">%s</p><div class="metric-grid">%s</div></div></section>`,
			html.EscapeString(page.ID), hiddenAttr, html.EscapeString(pageType), html.EscapeString(page.Title), html.EscapeString(page.Description), buildPreviewMetricCards(schema))
	case "list":
		return fmt.Sprintf(`<section class="page-section" data-page-id="%s"%s><div class="page-card"><p class="page-eyebrow">%s</p><h2>%s</h2><p class="page-desc">%s</p><div class="chip-row">%s</div><div class="table-shell">%s</div></div></section>`,
			html.EscapeString(page.ID), hiddenAttr, html.EscapeString(pageType), html.EscapeString(page.Title), html.EscapeString(page.Description), buildPreviewActionChips(page), buildPreviewTable(schema, page))
	case "form":
		return fmt.Sprintf(`<section class="page-section" data-page-id="%s"%s><div class="page-card"><p class="page-eyebrow">%s</p><h2>%s</h2><p class="page-desc">%s</p><div class="form-shell">%s</div></div></section>`,
			html.EscapeString(page.ID), hiddenAttr, html.EscapeString(pageType), html.EscapeString(page.Title), html.EscapeString(page.Description), buildPreviewForm(schema, page))
	default:
		return fmt.Sprintf(`<section class="page-section" data-page-id="%s"%s><div class="page-card"><p class="page-eyebrow">%s</p><h2>%s</h2><p class="page-desc">%s</p><div class="detail-grid">%s</div></div></section>`,
			html.EscapeString(page.ID), hiddenAttr, html.EscapeString(pageType), html.EscapeString(page.Title), html.EscapeString(page.Description), buildPreviewDetail(schema, page))
	}
}

func buildPreviewMetricCards(schema UISchema) string {
	raw := schema.MockData["dashboard_metrics"]
	if metricList, ok := raw.([]map[string]any); ok && len(metricList) > 0 {
		cards := make([]string, 0, len(metricList))
		for _, metric := range metricList {
			cards = append(cards, fmt.Sprintf(`<article class="metric-card"><div class="metric-label">%s</div><div class="metric-value">%s</div></article>`,
				html.EscapeString(readStringAny(metric["label"], "指标")),
				html.EscapeString(readStringAny(metric["value"], "0")),
			))
		}
		return strings.Join(cards, "")
	}
	if metricMap, ok := raw.(map[string]any); ok && len(metricMap) > 0 {
		cards := make([]string, 0, len(metricMap))
		for key, value := range metricMap {
			cards = append(cards, fmt.Sprintf(`<article class="metric-card"><div class="metric-label">%s</div><div class="metric-value">%v</div></article>`,
				html.EscapeString(strings.ReplaceAll(key, "_", " ")),
				value,
			))
		}
		return strings.Join(cards, "")
	}
	return `<article class="metric-card"><div class="metric-label">暂无指标</div><div class="metric-value">-</div></article>`
}

func buildPreviewActionChips(page UIPage) string {
	chips := make([]string, 0, len(page.PrimaryCTAs)+len(page.SecondaryCTAs))
	for _, item := range append(append([]string{}, page.PrimaryCTAs...), page.SecondaryCTAs...) {
		if strings.TrimSpace(item) == "" {
			continue
		}
		chips = append(chips, `<span class="action-chip">`+html.EscapeString(item)+`</span>`)
	}
	if len(chips) == 0 {
		return `<span class="action-chip muted">暂无动作</span>`
	}
	return strings.Join(chips, "")
}

func buildPreviewTable(schema UISchema, page UIPage) string {
	var columns []string
	for _, table := range schema.Tables {
		if table.PageID == page.ID || table.PageID == "" {
			columns = ensureSlice(table.Columns)
			if len(columns) > 0 {
				break
			}
		}
	}
	if len(columns) == 0 {
		columns = []string{"字段", "值", "状态"}
	}
	headerCells := make([]string, 0, len(columns))
	for _, column := range columns {
		headerCells = append(headerCells, `<th>`+html.EscapeString(column)+`</th>`)
	}
	rows := buildPreviewRows(schema, page, columns)
	return `<table><thead><tr>` + strings.Join(headerCells, "") + `</tr></thead><tbody>` + rows + `</tbody></table>`
}

func buildPreviewRows(schema UISchema, page UIPage, columns []string) string {
	records := inferPreviewRows(schema, page)
	if len(records) == 0 {
		return `<tr><td colspan="` + fmt.Sprintf("%d", len(columns)) + `">暂无示例数据</td></tr>`
	}
	rows := make([]string, 0, len(records))
	for _, record := range records {
		cells := make([]string, 0, len(columns))
		for _, column := range columns {
			cells = append(cells, `<td>`+html.EscapeString(readAnyValue(record, column))+`</td>`)
		}
		rows = append(rows, `<tr>`+strings.Join(cells, "")+`</tr>`)
	}
	return strings.Join(rows, "")
}

func buildPreviewForm(schema UISchema, page UIPage) string {
	for _, form := range schema.Forms {
		if form.PageID != page.ID && form.PageID != "" {
			continue
		}
		fields := make([]string, 0, len(form.Fields))
		for _, field := range form.Fields {
			fields = append(fields, `<label class="field-card"><span>`+html.EscapeString(field.Label)+`</span><div class="field-input">`+html.EscapeString(field.Placeholder)+`</div></label>`)
		}
		if len(fields) == 0 {
			break
		}
		return `<div class="field-grid">` + strings.Join(fields, "") + `</div><button class="submit-button">` + html.EscapeString(form.SubmitCTA) + `</button>`
	}
	return `<div class="empty-hint">当前表单字段尚未生成。</div>`
}

func buildPreviewDetail(schema UISchema, page UIPage) string {
	record := inferPreviewDetailRecord(schema, page)
	if len(record) == 0 {
		return `<div class="empty-hint">当前详情字段尚未生成。</div>`
	}
	cards := make([]string, 0, len(record))
	for key, value := range record {
		cards = append(cards, `<article class="detail-card"><div class="detail-label">`+html.EscapeString(key)+`</div><div class="detail-value">`+html.EscapeString(value)+`</div></article>`)
	}
	return strings.Join(cards, "")
}

func inferPreviewRows(schema UISchema, page UIPage) []map[string]any {
	haystack := strings.ToLower(page.Title + " " + page.Description)
	switch {
	case strings.Contains(haystack, "用户"):
		return readMapSliceAny(schema.MockData["users"])
	case strings.Contains(haystack, "角色"):
		return readMapSliceAny(schema.MockData["roles"])
	case strings.Contains(haystack, "导入"):
		return readMapSliceAny(schema.MockData["import_tasks"])
	case strings.Contains(haystack, "审计"), strings.Contains(haystack, "日志"):
		return readMapSliceAny(schema.MockData["audit_logs"])
	case strings.Contains(haystack, "组织"):
		return readMapSliceAny(schema.MockData["organizations"])
	default:
		return nil
	}
}

func inferPreviewDetailRecord(schema UISchema, page UIPage) map[string]string {
	rows := inferPreviewRows(schema, page)
	if len(rows) == 0 {
		return map[string]string{}
	}
	record := map[string]string{}
	for key, value := range rows[0] {
		record[key] = readStringAny(value, fmt.Sprintf("%v", value))
	}
	return record
}

func readMapSliceAny(value any) []map[string]any {
	items, ok := value.([]map[string]any)
	if ok {
		return items
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if mapped, ok := item.(map[string]any); ok {
			out = append(out, mapped)
		}
	}
	return out
}

func readAnyValue(record map[string]any, key string) string {
	if value, ok := record[key]; ok {
		return readStringAny(value, fmt.Sprintf("%v", value))
	}
	for recordKey, value := range record {
		if strings.EqualFold(recordKey, key) {
			return readStringAny(value, fmt.Sprintf("%v", value))
		}
	}
	return "-"
}

func readStringAny(value any, fallback string) string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			return typed
		}
	case fmt.Stringer:
		text := typed.String()
		if strings.TrimSpace(text) != "" {
			return text
		}
	case []string:
		if len(typed) > 0 {
			return strings.Join(typed, "、")
		}
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, readStringAny(item, ""))
		}
		return strings.Join(uniqueStrings(parts), "、")
	default:
		if value != nil {
			text := fmt.Sprintf("%v", value)
			if strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return fallback
}

func findUIPageByID(pages []UIPage, pageID string) UIPage {
	for _, page := range pages {
		if page.ID == pageID {
			return page
		}
	}
	if len(pages) > 0 {
		return pages[0]
	}
	return UIPage{ID: pageID, Title: pageID, Type: "detail"}
}

func prototypePreviewCSS() string {
	return `:root{color-scheme:light;font-family:"Avenir Next","IBM Plex Sans","Segoe UI",sans-serif;background:#efe4d2;color:#18202d}*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at top left,rgba(243,157,80,.26),transparent 28%),linear-gradient(180deg,#f5eee3 0%,#eadbc7 100%)}.preview-shell{min-height:100vh;display:grid;grid-template-columns:280px 1fr}.preview-sidebar{padding:28px;background:#18202d;color:#f7f1e8;display:flex;flex-direction:column;gap:18px}.preview-eyebrow{margin:0;text-transform:uppercase;letter-spacing:.16em;font-size:12px;opacity:.65}.preview-sidebar h1{margin:0;font-size:30px;line-height:1.15}.preview-description{margin:0;font-size:14px;line-height:1.8;color:rgba(247,241,232,.78)}.preview-nav{display:flex;flex-direction:column;gap:10px}.nav-item{border:none;border-radius:16px;padding:14px 16px;text-align:left;cursor:pointer;background:rgba(255,255,255,.08);color:inherit;font-size:15px}.nav-item.active{background:linear-gradient(135deg,#f29b52 0%,#ffd7ac 100%);color:#1d2129}.preview-main{padding:28px}.page-card{min-height:calc(100vh - 56px);background:rgba(255,252,246,.94);border:1px solid rgba(24,32,45,.07);border-radius:28px;padding:28px;box-shadow:0 24px 60px rgba(24,32,45,.1)}.page-eyebrow{text-transform:uppercase;letter-spacing:.16em;font-size:12px;color:rgba(24,32,45,.46)}.page-card h2{margin:8px 0 0;font-size:34px;line-height:1.15}.page-desc{margin:12px 0 24px;font-size:15px;line-height:1.8;color:rgba(24,32,45,.68)}.metric-grid,.detail-grid,.field-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:16px}.metric-card,.detail-card,.field-card{border-radius:22px;border:1px solid rgba(24,32,45,.08);background:#fffdf8;padding:18px}.metric-label,.detail-label,.field-card span{font-size:12px;text-transform:uppercase;letter-spacing:.14em;color:rgba(24,32,45,.42)}.metric-value,.detail-value{margin-top:10px;font-size:28px;font-weight:700;color:#18202d}.chip-row{display:flex;flex-wrap:wrap;gap:10px;margin-bottom:18px}.action-chip{display:inline-flex;align-items:center;border-radius:999px;background:#fff2e3;color:#93530d;padding:9px 14px;font-size:13px;font-weight:600}.action-chip.muted{background:#f2eee8;color:#5e626a}.table-shell{overflow:hidden;border-radius:24px;border:1px solid rgba(24,32,45,.08);background:#fffdf8}.table-shell table{width:100%;border-collapse:collapse}.table-shell th,.table-shell td{padding:14px 16px;border-bottom:1px solid rgba(24,32,45,.06);text-align:left;font-size:14px}.table-shell th{font-size:12px;text-transform:uppercase;letter-spacing:.12em;color:rgba(24,32,45,.42)}.form-shell{display:grid;gap:18px}.field-input{margin-top:10px;border-radius:16px;background:#f9f4ec;padding:14px;color:rgba(24,32,45,.58)}.submit-button{margin-top:10px;border:none;border-radius:18px;background:#18202d;color:#fff8ef;padding:14px 20px;font-size:15px;font-weight:700;cursor:pointer}.empty-hint{border-radius:20px;border:1px dashed rgba(24,32,45,.14);background:#fffaf2;padding:18px;color:rgba(24,32,45,.54)}@media (max-width:960px){.preview-shell{grid-template-columns:1fr}.preview-main{padding:18px}.page-card{padding:20px}}`
}
