package main

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestIntegration_ImportSourceMaterialFromFile_Markdown(t *testing.T) {
	ctx := context.Background()
	service, repo, tempDir := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Markdown Import", Description: "验证 markdown 导入"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	filePath := filepath.Join(tempDir, "customer-prd.md")
	if err := os.WriteFile(filePath, []byte("# 背景\n需要统一需求真相层。\n## 核心功能\n- PRD\n- 功能规格"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	material, err := service.ImportSourceMaterialFromFile(ctx, ImportSourceMaterialFromFileInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		FilePath:   filePath,
		IsPrimary:  true,
	})
	if err != nil {
		t.Fatalf("ImportSourceMaterialFromFile returned error: %v", err)
	}

	if material.FileName != "customer-prd.md" {
		t.Fatalf("expected imported file name, got %s", material.FileName)
	}
	if material.MimeType != "text/markdown" {
		t.Fatalf("expected markdown mime type, got %s", material.MimeType)
	}
	if sourceMaterialSectionCount(material) < 2 {
		t.Fatalf("expected parsed markdown sections, got %+v", material.ParsedContent)
	}
}

func TestIntegration_ImportSourceMaterialFromFile_DOCX(t *testing.T) {
	ctx := context.Background()
	service, repo, tempDir := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "DOCX Import", Description: "验证 docx 导入"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	filePath := filepath.Join(tempDir, "meeting-notes.docx")
	if err := writeTestDOCX(filePath, []string{"一、会议背景", "讨论需求澄清流程。", "二、决策", "先做 PRD 标准化。"}); err != nil {
		t.Fatalf("writeTestDOCX returned error: %v", err)
	}

	material, err := service.ImportSourceMaterialFromFile(ctx, ImportSourceMaterialFromFileInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypeMeetingNotes,
		FilePath:   filePath,
		IsPrimary:  false,
	})
	if err != nil {
		t.Fatalf("ImportSourceMaterialFromFile(docx) returned error: %v", err)
	}

	if material.MimeType != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("unexpected docx mime type: %s", material.MimeType)
	}
	if !strings.Contains(material.RawText, "会议背景") {
		t.Fatalf("expected docx text to be extracted, got %s", material.RawText)
	}
	if sourceMaterialSectionCount(material) < 2 {
		t.Fatalf("expected docx sections to be parsed, got %+v", material.ParsedContent)
	}
}

func TestIntegration_ImportSourceMaterialFromFile_PDF(t *testing.T) {
	ctx := context.Background()
	service, repo, tempDir := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "PDF Import", Description: "验证 pdf 导入"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	filePath := filepath.Join(tempDir, "customer-prd.pdf")
	if err := writeTestPDF(filePath, "PRD PDF DEMO"); err != nil {
		t.Fatalf("writeTestPDF returned error: %v", err)
	}

	material, err := service.ImportSourceMaterialFromFile(ctx, ImportSourceMaterialFromFileInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		FilePath:   filePath,
		IsPrimary:  false,
	})
	if err != nil {
		t.Fatalf("ImportSourceMaterialFromFile(pdf) returned error: %v", err)
	}

	if material.MimeType != "application/pdf" {
		t.Fatalf("unexpected pdf mime type: %s", material.MimeType)
	}
	if !strings.Contains(material.RawText, "PRD PDF DEMO") {
		t.Fatalf("expected pdf text extraction, got %s", material.RawText)
	}
}

func TestBlackBox_AppImportsSourceMaterialFromFile(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	project, err := app.CreateProject(CreateProjectInput{Name: "App File Import", Description: "验证 App 文件导入"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	filePath := filepath.Join(t.TempDir(), "chat-log.txt")
	if err := os.WriteFile(filePath, []byte("客户聊天记录\n需要先做澄清，再生成 PRD。"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	material, err := app.ImportSourceMaterialFromFile(ImportSourceMaterialFromFileInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypeChatLog,
		FilePath:   filePath,
		IsPrimary:  false,
	})
	if err != nil {
		t.Fatalf("ImportSourceMaterialFromFile returned error: %v", err)
	}

	if material.FileName != "chat-log.txt" {
		t.Fatalf("unexpected imported file name: %s", material.FileName)
	}
	if !strings.Contains(material.RawText, "客户聊天记录") {
		t.Fatalf("expected imported text content, got %s", material.RawText)
	}
}

func writeTestDOCX(path string, paragraphs []string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	zipWriter := zip.NewWriter(file)
	documentFile, err := zipWriter.Create("word/document.xml")
	if err != nil {
		return err
	}

	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	body.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, paragraph := range paragraphs {
		body.WriteString(`<w:p><w:r><w:t>`)
		body.WriteString(xmlEscape(paragraph))
		body.WriteString(`</w:t></w:r></w:p>`)
	}
	body.WriteString(`</w:body></w:document>`)

	if _, err := documentFile.Write([]byte(body.String())); err != nil {
		return err
	}
	return zipWriter.Close()
}

func writeTestPDF(path, text string) error {
	content := fmt.Sprintf("BT\n/F1 18 Tf\n72 100 Td\n(%s) Tj\nET", escapePDFText(text))
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}

	var builder strings.Builder
	builder.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = builder.Len()
		builder.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", index+1, object))
	}
	xrefOffset := builder.Len()
	builder.WriteString("xref\n0 6\n")
	builder.WriteString("0000000000 65535 f \n")
	for index := 1; index <= len(objects); index++ {
		builder.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[index]))
	}
	builder.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n")
	builder.WriteString(strconv.Itoa(xrefOffset))
	builder.WriteString("\n%%EOF\n")
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func escapePDFText(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
	return replacer.Replace(value)
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}
