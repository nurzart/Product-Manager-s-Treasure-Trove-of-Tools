package main

import (
	"regexp"
	"strings"
)

var (
	markdownHeadingPattern = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.+?)\s*$`)
	numberedHeadingPattern = regexp.MustCompile(`^\s*(\d+(?:\.\d+)*)(?:[、.])\s*(.+?)\s*$`)
	chineseHeadingPattern  = regexp.MustCompile(`^\s*([一二三四五六七八九十]+)[、.]\s*(.+?)\s*$`)
)

func enrichSourceMaterialInput(input SaveSourceMaterialInput) SaveSourceMaterialInput {
	input.ParsedContent = ensureMap(input.ParsedContent)
	input.ExtractionMeta = ensureMap(input.ExtractionMeta)

	rawText := strings.TrimSpace(input.RawText)
	if rawText == "" {
		return input
	}

	parsedContent, extractionMeta := parseSourceMaterialContent(input.SourceType, input.Title, rawText)
	for key, value := range parsedContent {
		input.ParsedContent[key] = value
	}
	for key, value := range extractionMeta {
		input.ExtractionMeta[key] = value
	}
	return input
}

func parseSourceMaterialContent(sourceType SourceMaterialType, title, rawText string) (map[string]any, map[string]any) {
	sections := extractStructuredSections(rawText)
	sectionTitles := make([]string, 0, len(sections))
	sectionViews := make([]map[string]any, 0, len(sections))
	for _, section := range sections {
		sectionTitles = append(sectionTitles, section.Title)
		sectionViews = append(sectionViews, map[string]any{
			"title":   section.Title,
			"level":   section.Level,
			"excerpt": clipText(section.Body, 180),
		})
	}

	keyPoints := extractKeyPoints(rawText)
	lineCount := countNonEmptyLines(rawText)
	characterCount := len([]rune(rawText))
	documentKind := inferDocumentKind(sourceType, title, rawText)

	parsedContent := map[string]any{
		"document_kind":   documentKind,
		"section_count":   len(sectionViews),
		"sections":        sectionViews,
		"headings":        ensureSlice(sectionTitles),
		"key_points":      keyPoints,
		"character_count": characterCount,
		"line_count":      lineCount,
	}
	if sourceType == SourceMaterialTypePRDUpload {
		parsedContent["prd_outline"] = buildPRDOutlineHints(sectionTitles)
		parsedContent["prd_baseline"] = buildPRDBaseline(sections, keyPoints)
	}

	extractionMeta := map[string]any{
		"parser":          "builtin-structure-parser",
		"parsed_at":       nowUTC().Format(timeLayout),
		"heading_count":   len(sectionViews),
		"key_point_count": len(keyPoints),
		"line_count":      lineCount,
		"character_count": characterCount,
		"is_structured":   len(sectionViews) > 0,
	}
	return parsedContent, extractionMeta
}

type parsedSection struct {
	Title string
	Body  string
	Level int
}

func extractStructuredSections(rawText string) []parsedSection {
	lines := strings.Split(strings.ReplaceAll(rawText, "\r\n", "\n"), "\n")
	sections := make([]parsedSection, 0)
	var current *parsedSection

	flushCurrent := func() {
		if current == nil {
			return
		}
		current.Body = strings.TrimSpace(current.Body)
		if current.Title != "" || current.Body != "" {
			sections = append(sections, *current)
		}
		current = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if title, level, ok := detectHeading(trimmed); ok {
			flushCurrent()
			current = &parsedSection{Title: title, Level: level}
			continue
		}
		if current == nil {
			if trimmed == "" {
				continue
			}
			current = &parsedSection{Title: "概述", Level: 1, Body: trimmed}
			continue
		}
		if trimmed == "" {
			if current.Body != "" && !strings.HasSuffix(current.Body, "\n") {
				current.Body += "\n"
			}
			continue
		}
		if current.Body != "" && !strings.HasSuffix(current.Body, "\n") {
			current.Body += "\n"
		}
		current.Body += trimmed
	}
	flushCurrent()

	if len(sections) == 1 && sections[0].Title == "概述" && sections[0].Body == strings.TrimSpace(rawText) {
		return nil
	}
	return sections
}

func detectHeading(line string) (string, int, bool) {
	if line == "" {
		return "", 0, false
	}
	if matches := markdownHeadingPattern.FindStringSubmatch(line); len(matches) == 3 {
		return strings.TrimSpace(matches[2]), len(matches[1]), true
	}
	if matches := numberedHeadingPattern.FindStringSubmatch(line); len(matches) == 3 {
		return strings.TrimSpace(matches[2]), strings.Count(matches[1], ".") + 1, true
	}
	if matches := chineseHeadingPattern.FindStringSubmatch(line); len(matches) == 3 {
		return strings.TrimSpace(matches[2]), 1, true
	}
	return "", 0, false
}

func extractKeyPoints(rawText string) []string {
	lines := strings.Split(strings.ReplaceAll(rawText, "\r\n", "\n"), "\n")
	points := make([]string, 0, 8)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "•") {
			points = append(points, strings.TrimSpace(strings.TrimLeft(trimmed, "-*• ")))
			continue
		}
		if _, _, ok := detectHeading(trimmed); ok {
			continue
		}
		if len(points) < 4 {
			points = append(points, clipText(trimmed, 80))
		}
		if len(points) >= 8 {
			break
		}
	}
	return uniqueStrings(points)
}

func buildPRDOutlineHints(sectionTitles []string) map[string]bool {
	joined := strings.ToLower(strings.Join(sectionTitles, "\n"))
	return map[string]bool{
		"background": strings.Contains(joined, "背景") || strings.Contains(joined, "背景与问题"),
		"goals":      strings.Contains(joined, "目标") || strings.Contains(joined, "成功指标"),
		"users":      strings.Contains(joined, "用户") || strings.Contains(joined, "角色"),
		"scope":      strings.Contains(joined, "范围") || strings.Contains(joined, "边界"),
		"features":   strings.Contains(joined, "功能") || strings.Contains(joined, "模块"),
		"acceptance": strings.Contains(joined, "验收"),
		"risks":      strings.Contains(joined, "风险") || strings.Contains(joined, "依赖"),
	}
}

type prdCanonicalSection struct {
	Key      string
	Label    string
	Keywords []string
}

var prdCanonicalSections = []prdCanonicalSection{
	{Key: "background", Label: "背景与问题", Keywords: []string{"背景", "问题", "现状", "痛点", "机会"}},
	{Key: "goals", Label: "目标与指标", Keywords: []string{"目标", "指标", "成功", "收益", "okr", "kpi"}},
	{Key: "users", Label: "用户与角色", Keywords: []string{"用户", "角色", "画像", "对象", "persona"}},
	{Key: "scope", Label: "范围与边界", Keywords: []string{"范围", "边界", "不做", "scope", "约束"}},
	{Key: "features", Label: "功能与流程", Keywords: []string{"功能", "模块", "流程", "方案", "页面", "交互"}},
	{Key: "acceptance", Label: "验收与交付", Keywords: []string{"验收", "交付", "标准", "里程碑", "测试"}},
	{Key: "risks", Label: "风险与依赖", Keywords: []string{"风险", "依赖", "前提", "假设", "限制"}},
}

func buildPRDBaseline(sections []parsedSection, keyPoints []string) map[string]any {
	type prdSectionAggregate struct {
		Titles     []string
		Contents   []string
		Confidence int
	}

	aggregates := map[string]*prdSectionAggregate{}
	for _, section := range sections {
		key, confidence := classifyPRDSection(section)
		if key == "" {
			continue
		}
		aggregate := aggregates[key]
		if aggregate == nil {
			aggregate = &prdSectionAggregate{}
			aggregates[key] = aggregate
		}
		if strings.TrimSpace(section.Title) != "" {
			aggregate.Titles = append(aggregate.Titles, strings.TrimSpace(section.Title))
		}
		if strings.TrimSpace(section.Body) != "" {
			aggregate.Contents = append(aggregate.Contents, strings.TrimSpace(section.Body))
		}
		if confidence > aggregate.Confidence {
			aggregate.Confidence = confidence
		}
	}

	sectionViews := make([]map[string]any, 0, len(prdCanonicalSections))
	presentKeys := make([]string, 0, len(prdCanonicalSections))
	missingKeys := make([]string, 0, len(prdCanonicalSections))
	for _, definition := range prdCanonicalSections {
		aggregate := aggregates[definition.Key]
		content := ""
		titles := []string{}
		confidence := 0
		status := "missing"
		if aggregate != nil {
			titles = uniqueStrings(aggregate.Titles)
			content = strings.TrimSpace(strings.Join(aggregate.Contents, "\n\n"))
			confidence = aggregate.Confidence
		}
		if content != "" {
			status = "present"
			presentKeys = append(presentKeys, definition.Key)
		} else {
			missingKeys = append(missingKeys, definition.Key)
		}
		sectionViews = append(sectionViews, map[string]any{
			"key":        definition.Key,
			"label":      definition.Label,
			"status":     status,
			"titles":     titles,
			"content":    content,
			"excerpt":    clipText(content, 140),
			"confidence": confidence,
		})
	}

	completenessScore := 0
	if len(prdCanonicalSections) > 0 {
		completenessScore = len(presentKeys) * 100 / len(prdCanonicalSections)
	}
	summaryParts := []string{
		"系统已将上传 PRD 归一为标准章节基线。",
	}
	if len(presentKeys) > 0 {
		summaryParts = append(summaryParts, "已覆盖 "+strings.Join(prdBaselineLabels(presentKeys), "、"))
	}
	if len(missingKeys) > 0 {
		summaryParts = append(summaryParts, "待补充 "+strings.Join(prdBaselineLabels(missingKeys), "、"))
	}

	return map[string]any{
		"sections":           sectionViews,
		"present_keys":       presentKeys,
		"missing_keys":       missingKeys,
		"completeness_score": completenessScore,
		"summary":            strings.Join(summaryParts, "；") + "。",
		"key_points":         keyPoints,
	}
}

func classifyPRDSection(section parsedSection) (string, int) {
	title := strings.ToLower(strings.TrimSpace(section.Title))
	body := strings.ToLower(strings.TrimSpace(section.Body))
	bestKey := ""
	bestScore := 0
	for _, definition := range prdCanonicalSections {
		score := 0
		for _, keyword := range definition.Keywords {
			normalizedKeyword := strings.ToLower(keyword)
			switch {
			case strings.Contains(title, normalizedKeyword):
				score += 6
			case strings.Contains(body, normalizedKeyword):
				score += 2
			}
		}
		if score > bestScore {
			bestKey = definition.Key
			bestScore = score
		}
	}
	switch {
	case bestScore >= 6:
		return bestKey, 95
	case bestScore >= 3:
		return bestKey, 72
	default:
		return "", 0
	}
}

func prdBaselineLabels(keys []string) []string {
	labels := make([]string, 0, len(keys))
	for _, key := range keys {
		if label := prdBaselineLabelForKey(key); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

func prdBaselineLabelForKey(key string) string {
	for _, definition := range prdCanonicalSections {
		if definition.Key == key {
			return definition.Label
		}
	}
	return ""
}

func countNonEmptyLines(rawText string) int {
	lines := strings.Split(strings.ReplaceAll(rawText, "\r\n", "\n"), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func inferDocumentKind(sourceType SourceMaterialType, title, rawText string) string {
	normalized := strings.ToLower(strings.Join([]string{string(sourceType), title, rawText}, "\n"))
	switch {
	case sourceType == SourceMaterialTypePRDUpload || strings.Contains(normalized, "prd"):
		return "prd"
	case sourceType == SourceMaterialTypeMeetingNotes || strings.Contains(normalized, "会议纪要"):
		return "meeting_notes"
	case sourceType == SourceMaterialTypeChatLog || strings.Contains(normalized, "聊天"):
		return "chat_log"
	case sourceType == SourceMaterialTypeBidDoc || strings.Contains(normalized, "招标"):
		return "bid_doc"
	default:
		return "generic_text"
	}
}

func sourceMaterialHeadingTitles(material SourceMaterial) []string {
	rawHeadings, ok := material.ParsedContent["headings"]
	if !ok {
		return []string{}
	}
	values, ok := rawHeadings.([]any)
	if ok {
		headings := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				headings = append(headings, strings.TrimSpace(text))
			}
		}
		return uniqueStrings(headings)
	}
	if values, ok := rawHeadings.([]string); ok {
		return uniqueStrings(values)
	}
	return []string{}
}

func sourceMaterialKeyPoints(material SourceMaterial) []string {
	rawPoints, ok := material.ParsedContent["key_points"]
	if !ok {
		return []string{}
	}
	values, ok := rawPoints.([]any)
	if ok {
		points := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				points = append(points, strings.TrimSpace(text))
			}
		}
		return uniqueStrings(points)
	}
	if values, ok := rawPoints.([]string); ok {
		return uniqueStrings(values)
	}
	return []string{}
}

func sourceMaterialDocumentKind(material SourceMaterial) string {
	if kind, ok := material.ParsedContent["document_kind"].(string); ok {
		return strings.TrimSpace(kind)
	}
	return ""
}

func sourceMaterialSectionCount(material SourceMaterial) int {
	switch value := material.ParsedContent["section_count"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return len(sourceMaterialHeadingTitles(material))
	}
}

func sourceMaterialPRDBaseline(material SourceMaterial) map[string]any {
	value, ok := material.ParsedContent["prd_baseline"].(map[string]any)
	if ok {
		return ensureMap(value)
	}
	return map[string]any{}
}

func sourceMaterialPRDBaselineCompleteness(material SourceMaterial) int {
	switch value := sourceMaterialPRDBaseline(material)["completeness_score"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func sourceMaterialPRDBaselineSections(material SourceMaterial) []map[string]any {
	rawSections, ok := sourceMaterialPRDBaseline(material)["sections"]
	if !ok {
		return []map[string]any{}
	}
	switch values := rawSections.(type) {
	case []map[string]any:
		return ensureSlice(values)
	case []any:
		sections := make([]map[string]any, 0, len(values))
		for _, value := range values {
			section, ok := value.(map[string]any)
			if !ok {
				continue
			}
			sections = append(sections, ensureMap(section))
		}
		return sections
	default:
		return []map[string]any{}
	}
}

func sourceMaterialPRDBaselineMarkdown(material SourceMaterial) string {
	baseline := sourceMaterialPRDBaseline(material)
	if len(baseline) == 0 {
		return strings.TrimSpace(material.RawText)
	}

	var lines []string
	if summary, ok := baseline["summary"].(string); ok && strings.TrimSpace(summary) != "" {
		lines = append(lines, "# PRD 标准化基线", strings.TrimSpace(summary))
	}
	for _, section := range sourceMaterialPRDBaselineSections(material) {
		status, _ := section["status"].(string)
		if status != "present" {
			continue
		}
		label, _ := section["label"].(string)
		content, _ := section["content"].(string)
		if strings.TrimSpace(label) == "" || strings.TrimSpace(content) == "" {
			continue
		}
		lines = append(lines, "## "+strings.TrimSpace(label), strings.TrimSpace(content))
	}
	if len(lines) == 0 {
		return strings.TrimSpace(material.RawText)
	}
	return strings.TrimSpace(strings.Join(lines, "\n\n"))
}

func mergePRDBaselineEdits(current map[string]any, edits []PRDBaselineSectionInput) map[string]any {
	current = ensureMap(current)
	editMap := map[string]string{}
	for _, section := range edits {
		key := strings.TrimSpace(section.Key)
		if key == "" {
			continue
		}
		editMap[key] = strings.TrimSpace(section.Content)
	}

	currentSections := map[string]map[string]any{}
	switch rawSections := current["sections"].(type) {
	case []map[string]any:
		for _, section := range rawSections {
			key, _ := section["key"].(string)
			if key != "" {
				currentSections[key] = ensureMap(section)
			}
		}
	case []any:
		for _, rawSection := range rawSections {
			section, ok := rawSection.(map[string]any)
			if !ok {
				continue
			}
			key, _ := section["key"].(string)
			if key != "" {
				currentSections[key] = ensureMap(section)
			}
		}
	}

	sectionViews := make([]map[string]any, 0, len(prdCanonicalSections))
	presentKeys := make([]string, 0, len(prdCanonicalSections))
	missingKeys := make([]string, 0, len(prdCanonicalSections))
	for _, definition := range prdCanonicalSections {
		currentSection := ensureMap(currentSections[definition.Key])
		titles := []string{}
		switch rawTitles := currentSection["titles"].(type) {
		case []string:
			titles = ensureSlice(rawTitles)
		case []any:
			for _, title := range rawTitles {
				if text, ok := title.(string); ok && strings.TrimSpace(text) != "" {
					titles = append(titles, strings.TrimSpace(text))
				}
			}
		}

		content := strings.TrimSpace(editMap[definition.Key])
		if content == "" {
			if currentContent, ok := currentSection["content"].(string); ok {
				content = strings.TrimSpace(currentContent)
			}
		}
		status := "missing"
		confidence := 0
		if content != "" {
			status = "present"
			confidence = 100
			presentKeys = append(presentKeys, definition.Key)
		} else {
			missingKeys = append(missingKeys, definition.Key)
		}
		sectionViews = append(sectionViews, map[string]any{
			"key":        definition.Key,
			"label":      definition.Label,
			"status":     status,
			"titles":     titles,
			"content":    content,
			"excerpt":    clipText(content, 140),
			"confidence": confidence,
		})
	}

	completenessScore := 0
	if len(prdCanonicalSections) > 0 {
		completenessScore = len(presentKeys) * 100 / len(prdCanonicalSections)
	}

	summaryParts := []string{
		"该 PRD 标准章节已完成一次人工确认。",
	}
	if len(presentKeys) > 0 {
		summaryParts = append(summaryParts, "已覆盖 "+strings.Join(prdBaselineLabels(presentKeys), "、"))
	}
	if len(missingKeys) > 0 {
		summaryParts = append(summaryParts, "待补充 "+strings.Join(prdBaselineLabels(missingKeys), "、"))
	}

	return map[string]any{
		"sections":           sectionViews,
		"present_keys":       presentKeys,
		"missing_keys":       missingKeys,
		"completeness_score": completenessScore,
		"summary":            strings.Join(summaryParts, "；") + "。",
		"key_points":         current["key_points"],
	}
}
