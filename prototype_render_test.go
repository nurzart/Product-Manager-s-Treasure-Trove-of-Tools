package main

import (
	"strings"
	"testing"
)

func TestParsePrototypeRenderBundleJSON(t *testing.T) {
	payload := `{
		"render_mode": "ai_original",
		"design_summary": "AI 生成了带顶部导航的页面。",
		"app_tsx": "export default function App(){ return <div>Hello</div>; }",
		"styles_css": "body { background: white; }",
		"preview_html": "<!doctype html><html><body>Hello</body></html>"
	}`

	spec, err := parsePrototypeRenderBundleJSON(payload)
	if err != nil {
		t.Fatalf("parsePrototypeRenderBundleJSON returned error: %v", err)
	}
	if spec.RenderMode != prototypeRenderModeAIOriginal {
		t.Fatalf("expected render mode %s, got %s", prototypeRenderModeAIOriginal, spec.RenderMode)
	}
	if !strings.Contains(spec.PreviewHTML, "Hello") {
		t.Fatalf("expected preview html to be parsed")
	}
}

func TestBuildFallbackPrototypeRenderBundle(t *testing.T) {
	project := Project{Name: "客户管理"}
	schema := UISchema{
		AppMeta: UIAppMeta{Name: "客户管理原型", Description: "desc"},
		Routes:  []UIRoute{{Path: "/", PageID: "dashboard", Label: "概览"}},
		Pages:   []UIPage{{ID: "dashboard", Title: "概览", Type: "dashboard", Description: "desc"}},
	}

	spec := buildFallbackPrototypeRenderBundle(project, schema)
	if spec.RenderMode != prototypeRenderModeFallback {
		t.Fatalf("expected fallback render mode, got %s", spec.RenderMode)
	}
	if strings.TrimSpace(spec.PreviewHTML) == "" {
		t.Fatalf("expected fallback preview html")
	}
	if !strings.Contains(spec.PreviewHTML, "客户管理原型") {
		t.Fatalf("expected preview html to include schema app name")
	}
}

func TestCachedPrototypeRenderBundleSpec(t *testing.T) {
	generationMeta := mustJSON(map[string]any{
		"prototype_render_bundle": PrototypeRenderBundleSpec{
			RenderMode:    prototypeRenderModeProjectRepair,
			DesignSummary: "已修复菜单点击问题。",
			AppTSX:        "export default function App(){ return <div>fixed</div>; }",
			StylesCSS:     "body { background: #fff; }",
			PreviewHTML:   "<!doctype html><html><body>fixed</body></html>",
		},
	})

	spec, ok := cachedPrototypeRenderBundleSpec(generationMeta)
	if !ok {
		t.Fatalf("expected cached prototype render bundle to be found")
	}
	if spec.RenderMode != prototypeRenderModeProjectRepair {
		t.Fatalf("expected cached render mode %s, got %s", prototypeRenderModeProjectRepair, spec.RenderMode)
	}
	if !strings.Contains(spec.PreviewHTML, "fixed") {
		t.Fatalf("expected cached preview html to be parsed")
	}
}

func TestDiagnosePrototypeRenderSpec_FlagsStaticNavigation(t *testing.T) {
	schema := UISchema{
		AppMeta: UIAppMeta{Name: "用户治理", Description: "desc"},
		Routes: []UIRoute{
			{Path: "/", PageID: "dashboard", Label: "概览"},
			{Path: "/users", PageID: "user-list", Label: "用户列表"},
		},
		Pages: []UIPage{
			{ID: "dashboard", Title: "概览", Type: "dashboard", Description: "desc"},
			{ID: "user-list", Title: "用户列表", Type: "list", Description: "desc"},
		},
	}

	diagnostics := diagnosePrototypeRenderSpec(schema, PrototypeRenderBundleSpec{
		RenderMode:  prototypeRenderModeAIOriginal,
		AppTSX:      "export default function App(){ return <div>mock</div>; }",
		PreviewHTML: "<!doctype html><html><body><div class='nav-item'>概览</div><div class='nav-item'>用户列表</div></body></html>",
	})

	if len(diagnostics) == 0 {
		t.Fatalf("expected static navigation diagnostics")
	}
	if diagnostics[0].Code == "" {
		t.Fatalf("expected diagnostic code to be set")
	}
}
