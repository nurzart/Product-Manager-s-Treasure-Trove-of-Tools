package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pdfreader "github.com/ledongthuc/pdf"
)

func (s *WorkbenchService) ImportSourceMaterialFromFile(ctx context.Context, input ImportSourceMaterialFromFileInput) (SourceMaterial, error) {
	filePath := strings.TrimSpace(input.FilePath)
	if filePath == "" {
		return SourceMaterial{}, errors.New("file path is required")
	}

	rawText, fileName, mimeType, err := readImportedSourceMaterial(filePath)
	if err != nil {
		return SourceMaterial{}, err
	}

	sourceType := input.SourceType
	if sourceType == "" || sourceType == SourceMaterialTypeOtherFile {
		sourceType = inferSourceMaterialTypeFromFilename(fileName)
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	}

	return s.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  input.ProjectID,
		SourceType: sourceType,
		Title:      title,
		RawText:    rawText,
		FileName:   fileName,
		MimeType:   mimeType,
		FilePath:   filePath,
		IsPrimary:  input.IsPrimary,
		ExtractionMeta: map[string]any{
			"import_method": "local_file",
		},
	})
}

func readImportedSourceMaterial(filePath string) (string, string, string, error) {
	extension := strings.ToLower(filepath.Ext(filePath))
	fileName := filepath.Base(filePath)

	switch extension {
	case ".txt", ".md", ".markdown", ".json", ".csv":
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", "", "", err
		}
		return normalizeImportedText(string(content)), fileName, inferMimeTypeFromExtension(extension), nil
	case ".docx":
		content, err := readDOCXText(filePath)
		if err != nil {
			return "", "", "", err
		}
		return content, fileName, inferMimeTypeFromExtension(extension), nil
	case ".pdf":
		content, err := readPDFText(filePath)
		if err != nil {
			return "", "", "", err
		}
		return content, fileName, inferMimeTypeFromExtension(extension), nil
	default:
		return "", "", "", fmt.Errorf("unsupported file type %s", extension)
	}
}

func readPDFText(filePath string) (string, error) {
	file, reader, err := pdfreader.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	plainText, err := reader.GetPlainText()
	if err != nil {
		return "", err
	}
	payload, err := io.ReadAll(plainText)
	if err != nil {
		return "", err
	}

	content := normalizeImportedText(string(payload))
	if content == "" {
		return "", errors.New("pdf text extraction returned empty content")
	}
	return content, nil
}

func readDOCXText(filePath string) (string, error) {
	archive, err := zip.OpenReader(filePath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = archive.Close()
	}()

	var contentParts []string
	for _, file := range archive.File {
		if !isDOCXContentFile(file.Name) {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return "", err
		}
		payload, readErr := io.ReadAll(handle)
		_ = handle.Close()
		if readErr != nil {
			return "", readErr
		}
		text, parseErr := extractDOCXXMLText(payload)
		if parseErr != nil {
			return "", parseErr
		}
		if strings.TrimSpace(text) != "" {
			contentParts = append(contentParts, text)
		}
	}

	content := normalizeImportedText(strings.Join(contentParts, "\n\n"))
	if content == "" {
		return "", errors.New("docx extraction returned empty content")
	}
	return content, nil
}

func isDOCXContentFile(name string) bool {
	return name == "word/document.xml" || strings.HasPrefix(name, "word/header") || strings.HasPrefix(name, "word/footer")
}

func extractDOCXXMLText(payload []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	var builder strings.Builder
	needsParagraphBreak := false

	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "tab":
				builder.WriteByte('\t')
			case "br", "cr":
				builder.WriteByte('\n')
			}
		case xml.EndElement:
			if typed.Name.Local == "p" {
				if needsParagraphBreak {
					builder.WriteString("\n\n")
					needsParagraphBreak = false
				}
			}
		case xml.CharData:
			text := strings.TrimSpace(string(typed))
			if text == "" {
				continue
			}
			builder.WriteString(text)
			needsParagraphBreak = true
		}
	}
	return builder.String(), nil
}

func normalizeImportedText(input string) string {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	cleaned := make([]string, 0, len(lines))
	blankCount := 0
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmed) == "" {
			blankCount++
			if blankCount > 1 {
				continue
			}
			cleaned = append(cleaned, "")
			continue
		}
		blankCount = 0
		cleaned = append(cleaned, trimmed)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func inferMimeTypeFromExtension(extension string) string {
	switch extension {
	case ".txt":
		return "text/plain"
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func inferSourceMaterialTypeFromFilename(fileName string) SourceMaterialType {
	normalized := strings.ToLower(fileName)
	switch {
	case strings.Contains(normalized, "prd"):
		return SourceMaterialTypePRDUpload
	case strings.Contains(normalized, "meeting"), strings.Contains(fileName, "会议纪要"):
		return SourceMaterialTypeMeetingNotes
	case strings.Contains(normalized, "chat"), strings.Contains(fileName, "聊天"):
		return SourceMaterialTypeChatLog
	case strings.Contains(normalized, "bid"), strings.Contains(fileName, "招标"):
		return SourceMaterialTypeBidDoc
	default:
		return SourceMaterialTypeOtherFile
	}
}
