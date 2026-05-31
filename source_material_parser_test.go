package main

import (
	"strings"
	"testing"
)

func TestUnit_EnrichSourceMaterialInput_ExtractsPRDStructure(t *testing.T) {
	input := enrichSourceMaterialInput(SaveSourceMaterialInput{
		ProjectID:  "project-1",
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户现有 PRD",
		RawText: strings.Join([]string{
			"# 背景与问题",
			"用户现在需要把模糊需求稳定转成文档。",
			"## 目标与指标",
			"20 分钟内生成 PRD 初稿。",
			"## 核心功能",
			"- 需求澄清",
			"- 功能规格",
		}, "\n"),
		IsPrimary: true,
	})

	if input.ParsedContent["document_kind"] != "prd" {
		t.Fatalf("expected document kind prd, got %+v", input.ParsedContent["document_kind"])
	}
	if sectionCount := sourceMaterialSectionCount(SourceMaterial{ParsedContent: input.ParsedContent}); sectionCount != 3 {
		t.Fatalf("expected 3 parsed sections, got %d", sectionCount)
	}
	headings := sourceMaterialHeadingTitles(SourceMaterial{ParsedContent: input.ParsedContent})
	if len(headings) != 3 || headings[0] != "背景与问题" {
		t.Fatalf("unexpected headings: %+v", headings)
	}
	if input.ExtractionMeta["parser"] != "builtin-structure-parser" {
		t.Fatalf("expected parser metadata to be set, got %+v", input.ExtractionMeta)
	}

	outline, ok := input.ParsedContent["prd_outline"].(map[string]bool)
	if !ok || !outline["background"] || !outline["features"] {
		t.Fatalf("expected prd outline hints to be set, got %+v", input.ParsedContent["prd_outline"])
	}

	material := SourceMaterial{ParsedContent: input.ParsedContent}
	baseline := sourceMaterialPRDBaseline(material)
	if sourceMaterialPRDBaselineCompleteness(material) <= 0 {
		t.Fatalf("expected prd baseline completeness to be calculated, got %+v", baseline)
	}
	sections := sourceMaterialPRDBaselineSections(material)
	if len(sections) != 7 {
		t.Fatalf("expected 7 canonical prd baseline sections, got %d", len(sections))
	}
	if sections[0]["key"] != "background" || sections[0]["status"] != "present" {
		t.Fatalf("expected first baseline section to map background, got %+v", sections[0])
	}
	if markdown := sourceMaterialPRDBaselineMarkdown(material); !strings.Contains(markdown, "## 背景与问题") {
		t.Fatalf("expected baseline markdown to render canonical sections, got %s", markdown)
	}
}

func TestUnit_ExtractStructuredSections_SupportsChineseNumberedHeadings(t *testing.T) {
	sections := extractStructuredSections(strings.Join([]string{
		"一、项目背景",
		"这是项目背景。",
		"二、范围边界",
		"这里只做个人版。",
	}, "\n"))

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].Title != "项目背景" || sections[1].Title != "范围边界" {
		t.Fatalf("unexpected section titles: %+v", sections)
	}
}
