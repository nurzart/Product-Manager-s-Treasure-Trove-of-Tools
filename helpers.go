package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

func nowUTC() time.Time {
	return time.Now().UTC().Round(time.Second)
}

func mustJSON(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func ensureSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func ensureMap[K comparable, V any](value map[K]V) map[K]V {
	if value == nil {
		return map[K]V{}
	}
	return value
}

func slugify(input string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(input)) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func parseDocumentJSON(payload string) (ArtifactDocument, error) {
	var document ArtifactDocument
	if strings.TrimSpace(payload) == "" {
		return document, errors.New("empty document payload")
	}
	if err := json.Unmarshal([]byte(payload), &document); err == nil {
		document = normalizeArtifactDocument(document)
		document = ensureArtifactDocumentDefaults(document)
		if err := document.Validate(); err == nil {
			return document, nil
		}
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return document, err
	}
	document = ensureArtifactDocumentDefaults(normalizeArtifactDocument(coerceArtifactDocument(raw)))
	return document, document.Validate()
}

func parseClarificationPayload(payload string) (RequirementSummary, []ClarificationQuestion, error) {
	if strings.TrimSpace(payload) == "" {
		return RequirementSummary{}, nil, errors.New("empty clarification payload")
	}

	var strict struct {
		Summary   RequirementSummary      `json:"summary"`
		Questions []ClarificationQuestion `json:"questions"`
	}
	if err := json.Unmarshal([]byte(payload), &strict); err == nil && len(strict.Questions) > 0 {
		return normalizeRequirementSummary(strict.Summary), ensureSlice(strict.Questions), nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return RequirementSummary{}, nil, err
	}

	questions := coerceClarificationQuestions(firstNonNil(raw["questions"], raw["clarifications"]))
	if len(questions) == 0 {
		return RequirementSummary{}, nil, errors.New("clarification questions were empty")
	}
	return normalizeRequirementSummary(coerceRequirementSummary(raw["summary"])), questions, nil
}

func parseUISchemaJSON(payload string) (UISchema, error) {
	var schema UISchema
	if strings.TrimSpace(payload) == "" {
		return schema, errors.New("empty ui schema payload")
	}
	if err := json.Unmarshal([]byte(payload), &schema); err == nil {
		schema = normalizeUISchema(ensureUISchemaDefaults(schema))
		if err := schema.Validate(); err == nil {
			return schema, nil
		}
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return schema, err
	}
	schema = normalizeUISchema(ensureUISchemaDefaults(coerceUISchema(raw)))
	return schema, schema.Validate()
}

func renderDocumentMarkdown(document ArtifactDocument) string {
	var sections []string
	sections = append(sections, "# "+document.Title)
	if document.Summary != "" {
		sections = append(sections, document.Summary)
	}
	for _, section := range document.Sections {
		body := strings.TrimSpace(section.Body)
		if body == "" {
			body = "_待补充_"
		}
		sections = append(sections, "## "+section.Title+"\n"+body)
	}
	if len(document.Highlights) > 0 {
		sections = append(sections, "## 亮点\n- "+strings.Join(document.Highlights, "\n- "))
	}
	if len(document.OpenQuestions) > 0 {
		sections = append(sections, "## 待确认\n- "+strings.Join(document.OpenQuestions, "\n- "))
	}
	return strings.Join(sections, "\n\n")
}

func renderDocumentPlainText(document ArtifactDocument) string {
	var parts []string
	parts = append(parts, document.Title)
	if document.Summary != "" {
		parts = append(parts, document.Summary)
	}
	for _, section := range document.Sections {
		parts = append(parts, section.Title)
		parts = append(parts, section.Body)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func extractJSONBlock(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", errors.New("empty response")
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}
	codeFence := regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*\\}|\\[.*\\])\\s*```")
	if matches := codeFence.FindStringSubmatch(trimmed); len(matches) > 1 && json.Valid([]byte(matches[1])) {
		return matches[1], nil
	}
	firstBrace := strings.IndexAny(trimmed, "{[")
	lastBrace := strings.LastIndexAny(trimmed, "}]")
	if firstBrace >= 0 && lastBrace > firstBrace {
		candidate := trimmed[firstBrace : lastBrace+1]
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}
	if candidate, ok := findFirstValidJSONBlock(trimmed); ok {
		return candidate, nil
	}
	return "", fmt.Errorf("response does not contain valid json")
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeProjects(projects []Project) []Project {
	return ensureSlice(projects)
}

func normalizeRequirementSummary(summary RequirementSummary) RequirementSummary {
	summary.TargetUsers = ensureSlice(summary.TargetUsers)
	summary.BusinessGoals = ensureSlice(summary.BusinessGoals)
	summary.CoreScenarios = ensureSlice(summary.CoreScenarios)
	summary.Constraints = ensureSlice(summary.Constraints)
	summary.OpenQuestions = ensureSlice(summary.OpenQuestions)
	return summary
}

func normalizeRequirementInput(requirement RequirementInput) RequirementInput {
	requirement.Summary = normalizeRequirementSummary(requirement.Summary)
	requirement.Clarifications = ensureSlice(requirement.Clarifications)
	requirement.Answers = ensureSlice(requirement.Answers)
	return requirement
}

func normalizeArtifactDocument(document ArtifactDocument) ArtifactDocument {
	document.Sections = ensureSlice(document.Sections)
	document.Highlights = ensureSlice(document.Highlights)
	document.OpenQuestions = ensureSlice(document.OpenQuestions)
	document.Metadata = ensureMap(document.Metadata)
	return document
}

func ensureArtifactDocumentDefaults(document ArtifactDocument) ArtifactDocument {
	if strings.TrimSpace(document.Title) == "" {
		switch document.ArtifactType {
		case ArtifactTypePRD:
			document.Title = "PRD"
		case ArtifactTypeFunctionalSpec:
			document.Title = "功能规格说明"
		case ArtifactTypeTechnicalSpec:
			document.Title = "研发实现说明"
		case ArtifactTypePrototype:
			document.Title = "UISchema"
		default:
			document.Title = "Generated Document"
		}
	}
	return document
}

func normalizeArtifactBundle(bundle ArtifactBundle) ArtifactBundle {
	bundle.Versions = ensureSlice(bundle.Versions)
	return bundle
}

func normalizeArtifactBundles(bundles []ArtifactBundle) []ArtifactBundle {
	bundles = ensureSlice(bundles)
	for index := range bundles {
		bundles[index] = normalizeArtifactBundle(bundles[index])
	}
	return bundles
}

func normalizeUISchema(schema UISchema) UISchema {
	schema.Routes = ensureSlice(schema.Routes)
	schema.Pages = ensureSlice(schema.Pages)
	schema.Components = ensureSlice(schema.Components)
	schema.Forms = ensureSlice(schema.Forms)
	schema.Tables = ensureSlice(schema.Tables)
	schema.Actions = ensureSlice(schema.Actions)
	schema.MockData = ensureMap(schema.MockData)
	schema.StateVariants = ensureSlice(schema.StateVariants)
	schema.NavigationMap = ensureSlice(schema.NavigationMap)

	for index := range schema.Pages {
		schema.Pages[index].Components = ensureSlice(schema.Pages[index].Components)
		schema.Pages[index].States = ensureSlice(schema.Pages[index].States)
		schema.Pages[index].PrimaryCTAs = ensureSlice(schema.Pages[index].PrimaryCTAs)
		schema.Pages[index].SecondaryCTAs = ensureSlice(schema.Pages[index].SecondaryCTAs)
	}
	for index := range schema.Forms {
		schema.Forms[index].Fields = ensureSlice(schema.Forms[index].Fields)
		schema.Forms[index].States = ensureSlice(schema.Forms[index].States)
		for fieldIndex := range schema.Forms[index].Fields {
			schema.Forms[index].Fields[fieldIndex].Options = ensureSlice(schema.Forms[index].Fields[fieldIndex].Options)
		}
	}
	for index := range schema.Tables {
		schema.Tables[index].Columns = ensureSlice(schema.Tables[index].Columns)
	}

	return schema
}

func normalizePrototypeBundle(bundle PrototypeBundle) PrototypeBundle {
	bundle.UISchema = normalizeUISchema(bundle.UISchema)
	bundle.Diagnostics = ensureSlice(bundle.Diagnostics)
	if bundle.RepairReport != nil {
		report := *bundle.RepairReport
		report.Diagnostics = ensureSlice(report.Diagnostics)
		bundle.RepairReport = &report
	}
	bundle.Files = ensureSlice(bundle.Files)
	if strings.TrimSpace(bundle.RenderMode) == "" {
		bundle.RenderMode = "fallback_template"
	}
	return bundle
}

func ensureUISchemaDefaults(schema UISchema) UISchema {
	if strings.TrimSpace(schema.AppMeta.Name) == "" {
		schema.AppMeta.Name = "Generated Prototype"
	}
	if len(schema.Routes) == 0 {
		schema.Routes = deriveRoutesFromPages(schema.Pages)
	}
	return schema
}

func normalizeWorkspaceSnapshot(workspace WorkspaceSnapshot) WorkspaceSnapshot {
	if workspace.RequirementInput != nil {
		requirement := normalizeRequirementInput(*workspace.RequirementInput)
		workspace.RequirementInput = &requirement
	}
	workspace.SourceMaterials = normalizeSourceMaterials(workspace.SourceMaterials)
	if workspace.RequirementIntake != nil {
		intake := normalizeRequirementIntake(*workspace.RequirementIntake)
		workspace.RequirementIntake = &intake
	}
	if workspace.ConsensusBrief != nil {
		brief := normalizeConsensusBrief(*workspace.ConsensusBrief)
		workspace.ConsensusBrief = &brief
	}
	if workspace.RequirementModel != nil {
		model := normalizeRequirementModel(*workspace.RequirementModel)
		workspace.RequirementModel = &model
	}
	workspace.Artifacts = normalizeArtifactBundles(workspace.Artifacts)
	workspace.WorkflowRuns = ensureSlice(workspace.WorkflowRuns)
	return workspace
}

func coerceArtifactDocument(raw map[string]any) ArtifactDocument {
	sections := coerceArtifactSections(raw["sections"])
	if len(sections) == 0 {
		sections = coerceArtifactSectionsFromTopLevel(raw)
	}
	summary := coerceString(raw["summary"])
	if len(sections) == 0 && summary != "" {
		sections = []ArtifactSection{
			{
				ID:    "summary",
				Title: "Summary",
				Body:  summary,
				HTML:  "<p>" + html.EscapeString(summary) + "</p>",
			},
		}
	}

	return ArtifactDocument{
		ArtifactType:  ArtifactType(coerceString(raw["artifactType"])),
		Title:         coerceString(raw["title"]),
		Summary:       summary,
		Sections:      sections,
		Highlights:    coerceStringSlice(raw["highlights"]),
		OpenQuestions: coerceStringSlice(raw["openQuestions"]),
		Metadata:      coerceStringMapAny(raw["metadata"]),
	}
}

func coerceRequirementSummary(value any) RequirementSummary {
	switch typed := value.(type) {
	case map[string]any:
		return RequirementSummary{
			ProductName:   coerceString(firstNonNil(typed["productName"], typed["product_name"], typed["name"], typed["title"])),
			TargetUsers:   coerceStringSlice(firstNonNil(typed["targetUsers"], typed["target_users"], typed["users"])),
			BusinessGoals: coerceStringSlice(firstNonNil(typed["businessGoals"], typed["business_goals"], typed["goals"])),
			CoreScenarios: coerceStringSlice(firstNonNil(typed["coreScenarios"], typed["core_scenarios"], typed["scenarios"])),
			Constraints:   coerceStringSlice(typed["constraints"]),
			OpenQuestions: coerceStringSlice(firstNonNil(typed["openQuestions"], typed["open_questions"])),
		}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return RequirementSummary{}
		}
		return RequirementSummary{
			BusinessGoals: []string{text},
		}
	default:
		text := coerceString(typed)
		if text == "" {
			return RequirementSummary{}
		}
		return RequirementSummary{
			BusinessGoals: []string{text},
		}
	}
}

func coerceUISchema(raw map[string]any) UISchema {
	return UISchema{
		AppMeta:       coerceUIAppMeta(firstNonNil(raw["app_meta"], raw["appMeta"])),
		Routes:        coerceUIRoutes(raw["routes"]),
		Pages:         coerceUIPages(raw["pages"]),
		Components:    coerceUIComponents(raw["components"]),
		Forms:         coerceUIForms(raw["forms"]),
		Tables:        coerceUITables(raw["tables"]),
		Actions:       coerceUIActions(raw["actions"]),
		MockData:      coerceStringMapAny(firstNonNil(raw["mock_data"], raw["mockData"])),
		StateVariants: coerceUIStateVariants(firstNonNil(raw["state_variants"], raw["stateVariants"])),
		NavigationMap: coerceUINavigations(firstNonNil(raw["navigation_map"], raw["navigationMap"])),
	}
}

func coerceClarificationQuestions(value any) []ClarificationQuestion {
	items, ok := value.([]any)
	if !ok {
		if text := coerceString(value); text != "" {
			return []ClarificationQuestion{
				{
					ID:        "clarification-1",
					Question:  text,
					Rationale: "由模型返回的自由文本追问转换而来。",
					Priority:  "medium",
				},
			}
		}
		return []ClarificationQuestion{}
	}

	questions := make([]ClarificationQuestion, 0, len(items))
	for index, item := range items {
		questionMap, ok := item.(map[string]any)
		if !ok {
			text := coerceString(item)
			if text == "" {
				continue
			}
			questions = append(questions, ClarificationQuestion{
				ID:        fmt.Sprintf("clarification-%d", index+1),
				Question:  text,
				Rationale: "由模型返回的自由文本追问转换而来。",
				Priority:  "medium",
			})
			continue
		}

		questions = append(questions, ClarificationQuestion{
			ID: firstNonEmpty(
				coerceString(questionMap["id"]),
				fmt.Sprintf("clarification-%d", index+1),
			),
			Question: firstNonEmpty(
				coerceString(questionMap["question"]),
				coerceString(questionMap["text"]),
				coerceString(questionMap["title"]),
				coerceString(questionMap["label"]),
			),
			Rationale: firstNonEmpty(
				coerceString(questionMap["rationale"]),
				coerceString(questionMap["reason"]),
				coerceString(questionMap["why"]),
				"帮助补齐需求边界与上下文。",
			),
			Priority: firstNonEmpty(
				coerceString(questionMap["priority"]),
				"medium",
			),
			Answer: coerceString(questionMap["answer"]),
		})
	}
	return questions
}

func coerceUIAppMeta(value any) UIAppMeta {
	typed, ok := value.(map[string]any)
	if !ok {
		name := coerceString(value)
		if name == "" {
			name = "Generated Prototype"
		}
		return UIAppMeta{Name: name}
	}
	return UIAppMeta{
		Name:        firstNonEmpty(coerceString(typed["name"]), "Generated Prototype"),
		Description: coerceString(firstNonNil(typed["description"], typed["summary"])),
	}
}

func coerceUIRoutes(value any) []UIRoute {
	entries := coerceKeyedEntries(value)
	routes := make([]UIRoute, 0, len(entries))
	for index, entry := range entries {
		routeMap, ok := entry.Value.(map[string]any)
		if ok {
			path := firstNonEmpty(
				coerceString(routeMap["path"]),
				defaultRoutePath(entry.Key, index),
			)
			pageID := firstNonEmpty(
				coerceString(firstNonNil(routeMap["page_id"], routeMap["pageId"], routeMap["id"])),
				entry.Key,
			)
			label := firstNonEmpty(
				coerceString(routeMap["label"]),
				coerceString(routeMap["title"]),
				pageID,
			)
			if path != "" && pageID != "" {
				routes = append(routes, UIRoute{Path: path, PageID: pageID, Label: label})
			}
			continue
		}

		pageID := firstNonEmpty(entry.Key, coerceString(entry.Value))
		if pageID == "" {
			continue
		}
		routes = append(routes, UIRoute{
			Path:   defaultRoutePath(pageID, index),
			PageID: pageID,
			Label:  pageID,
		})
	}
	return routes
}

func coerceUIPages(value any) []UIPage {
	entries := coerceKeyedEntries(value)
	pages := make([]UIPage, 0, len(entries))
	for index, entry := range entries {
		pageMap, ok := entry.Value.(map[string]any)
		if ok {
			pageID := firstNonEmpty(
				coerceString(firstNonNil(pageMap["id"], pageMap["page_id"], pageMap["pageId"])),
				entry.Key,
				fmt.Sprintf("page-%d", index+1),
			)
			pages = append(pages, UIPage{
				ID:            pageID,
				Title:         firstNonEmpty(coerceString(pageMap["title"]), coerceString(pageMap["label"]), pageID),
				Type:          firstNonEmpty(coerceString(pageMap["type"]), "detail"),
				Description:   coerceString(firstNonNil(pageMap["description"], pageMap["summary"])),
				Layout:        coerceString(pageMap["layout"]),
				Components:    coerceStringSlice(firstNonNil(pageMap["components"], pageMap["component_ids"])),
				States:        coerceStringSlice(pageMap["states"]),
				PrimaryCTAs:   coerceStringSlice(firstNonNil(pageMap["primary_ctas"], pageMap["primaryCTAs"])),
				SecondaryCTAs: coerceStringSlice(firstNonNil(pageMap["secondary_ctas"], pageMap["secondaryCTAs"])),
			})
			continue
		}

		title := coerceString(entry.Value)
		pageID := firstNonEmpty(entry.Key, fmt.Sprintf("page-%d", index+1))
		pages = append(pages, UIPage{
			ID:          pageID,
			Title:       firstNonEmpty(title, pageID),
			Type:        "detail",
			Description: "",
		})
	}
	return pages
}

func coerceUIComponents(value any) []UIComponent {
	entries := coerceKeyedEntries(value)
	components := make([]UIComponent, 0, len(entries))
	for index, entry := range entries {
		componentMap, ok := entry.Value.(map[string]any)
		if !ok {
			label := coerceString(entry.Value)
			if label == "" {
				continue
			}
			components = append(components, UIComponent{
				ID:    firstNonEmpty(entry.Key, fmt.Sprintf("component-%d", index+1)),
				Type:  "block",
				Label: label,
				Props: map[string]any{},
			})
			continue
		}

		components = append(components, UIComponent{
			ID: firstNonEmpty(
				coerceString(componentMap["id"]),
				entry.Key,
				fmt.Sprintf("component-%d", index+1),
			),
			Type:        firstNonEmpty(coerceString(componentMap["type"]), "block"),
			PageID:      coerceString(firstNonNil(componentMap["page_id"], componentMap["pageId"])),
			Label:       firstNonEmpty(coerceString(componentMap["label"]), coerceString(componentMap["title"]), entry.Key),
			Description: coerceString(componentMap["description"]),
			Props:       coerceStringMapAny(componentMap["props"]),
		})
	}
	return components
}

func coerceUIForms(value any) []UIForm {
	entries := coerceKeyedEntries(value)
	forms := make([]UIForm, 0, len(entries))
	for index, entry := range entries {
		formMap, ok := entry.Value.(map[string]any)
		if !ok {
			continue
		}
		forms = append(forms, UIForm{
			ID:        firstNonEmpty(coerceString(formMap["id"]), entry.Key, fmt.Sprintf("form-%d", index+1)),
			PageID:    coerceString(firstNonNil(formMap["page_id"], formMap["pageId"])),
			Title:     firstNonEmpty(coerceString(formMap["title"]), entry.Key),
			Fields:    coerceUIFields(formMap["fields"]),
			SubmitCTA: firstNonEmpty(coerceString(firstNonNil(formMap["submit_cta"], formMap["submitCTA"])), "提交"),
			States:    coerceStringSlice(formMap["states"]),
		})
	}
	return forms
}

func coerceUIFields(value any) []UIField {
	entries := coerceKeyedEntries(value)
	fields := make([]UIField, 0, len(entries))
	for index, entry := range entries {
		fieldMap, ok := entry.Value.(map[string]any)
		if !ok {
			label := coerceString(entry.Value)
			if label == "" {
				continue
			}
			fields = append(fields, UIField{
				ID:    firstNonEmpty(entry.Key, fmt.Sprintf("field-%d", index+1)),
				Label: label,
				Type:  "text",
			})
			continue
		}
		fields = append(fields, UIField{
			ID:          firstNonEmpty(coerceString(fieldMap["id"]), entry.Key, fmt.Sprintf("field-%d", index+1)),
			Label:       firstNonEmpty(coerceString(fieldMap["label"]), entry.Key),
			Type:        firstNonEmpty(coerceString(fieldMap["type"]), "text"),
			Placeholder: coerceString(fieldMap["placeholder"]),
			Required:    coerceBool(fieldMap["required"]),
			Options:     coerceStringSlice(fieldMap["options"]),
		})
	}
	return fields
}

func coerceUITables(value any) []UITable {
	entries := coerceKeyedEntries(value)
	tables := make([]UITable, 0, len(entries))
	for index, entry := range entries {
		tableMap, ok := entry.Value.(map[string]any)
		if !ok {
			continue
		}
		tables = append(tables, UITable{
			ID:      firstNonEmpty(coerceString(tableMap["id"]), entry.Key, fmt.Sprintf("table-%d", index+1)),
			PageID:  coerceString(firstNonNil(tableMap["page_id"], tableMap["pageId"])),
			Title:   firstNonEmpty(coerceString(tableMap["title"]), entry.Key),
			Columns: coerceStringSlice(tableMap["columns"]),
		})
	}
	return tables
}

func coerceUIActions(value any) []UIAction {
	entries := coerceKeyedEntries(value)
	actions := make([]UIAction, 0, len(entries))
	for index, entry := range entries {
		actionMap, ok := entry.Value.(map[string]any)
		if !ok {
			continue
		}
		actions = append(actions, UIAction{
			ID:           firstNonEmpty(coerceString(actionMap["id"]), entry.Key, fmt.Sprintf("action-%d", index+1)),
			Label:        firstNonEmpty(coerceString(actionMap["label"]), coerceString(actionMap["title"]), entry.Key),
			Kind:         firstNonEmpty(coerceString(actionMap["kind"]), "navigate"),
			SourcePageID: coerceString(firstNonNil(actionMap["source_page_id"], actionMap["sourcePageID"], actionMap["sourcePageId"])),
			TargetPageID: coerceString(firstNonNil(actionMap["target_page_id"], actionMap["targetPageID"], actionMap["targetPageId"])),
			Description:  coerceString(actionMap["description"]),
		})
	}
	return actions
}

func coerceUIStateVariants(value any) []UIStateVariant {
	entries := coerceKeyedEntries(value)
	variants := make([]UIStateVariant, 0, len(entries))
	for _, entry := range entries {
		variantMap, ok := entry.Value.(map[string]any)
		if !ok {
			continue
		}
		variants = append(variants, UIStateVariant{
			PageID: coerceString(firstNonNil(variantMap["page_id"], variantMap["pageId"])),
			State:  coerceString(variantMap["state"]),
			Notes:  coerceString(variantMap["notes"]),
		})
	}
	return variants
}

func coerceUINavigations(value any) []UINavigation {
	entries := coerceKeyedEntries(value)
	navigations := make([]UINavigation, 0, len(entries))
	for _, entry := range entries {
		navMap, ok := entry.Value.(map[string]any)
		if !ok {
			continue
		}
		navigations = append(navigations, UINavigation{
			FromPageID: coerceString(firstNonNil(navMap["from_page_id"], navMap["fromPageID"], navMap["fromPageId"])),
			ToPageID:   coerceString(firstNonNil(navMap["to_page_id"], navMap["toPageID"], navMap["toPageId"])),
			Trigger:    coerceString(navMap["trigger"]),
		})
	}
	return navigations
}

type keyedEntry struct {
	Key   string
	Value any
}

func coerceKeyedEntries(value any) []keyedEntry {
	switch typed := value.(type) {
	case []any:
		entries := make([]keyedEntry, 0, len(typed))
		for _, item := range typed {
			entries = append(entries, keyedEntry{Value: item})
		}
		return entries
	case map[string]any:
		entries := make([]keyedEntry, 0, len(typed))
		for key, item := range typed {
			entries = append(entries, keyedEntry{Key: key, Value: item})
		}
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Key < entries[j].Key
		})
		return entries
	default:
		return nil
	}
}

func deriveRoutesFromPages(pages []UIPage) []UIRoute {
	routes := make([]UIRoute, 0, len(pages))
	for index, page := range pages {
		if strings.TrimSpace(page.ID) == "" {
			continue
		}
		routes = append(routes, UIRoute{
			Path:   defaultRoutePath(page.ID, index),
			PageID: page.ID,
			Label:  firstNonEmpty(page.Title, page.ID),
		})
	}
	return routes
}

func defaultRoutePath(pageID string, index int) string {
	pageID = strings.TrimSpace(pageID)
	if index == 0 || pageID == "" || pageID == "dashboard" || pageID == "home" {
		return "/"
	}
	return "/" + slugify(pageID)
}

func coerceBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		default:
			return false
		}
	case float64:
		return typed != 0
	default:
		return false
	}
}

func coerceArtifactSections(value any) []ArtifactSection {
	items, ok := value.([]any)
	if !ok {
		return []ArtifactSection{}
	}

	sections := make([]ArtifactSection, 0, len(items))
	for index, item := range items {
		sectionMap, ok := item.(map[string]any)
		if !ok {
			body := coerceString(item)
			if body == "" {
				continue
			}
			sections = append(sections, ArtifactSection{
				ID:    fmt.Sprintf("section-%d", index+1),
				Title: fmt.Sprintf("Section %d", index+1),
				Body:  body,
				HTML:  "<p>" + html.EscapeString(body) + "</p>",
			})
			continue
		}

		title := firstNonEmpty(
			coerceString(sectionMap["title"]),
			coerceString(sectionMap["heading"]),
			coerceString(sectionMap["name"]),
			coerceString(sectionMap["label"]),
		)
		body := firstNonEmpty(
			coerceString(sectionMap["body"]),
			coerceString(sectionMap["content"]),
			coerceString(sectionMap["text"]),
			coerceString(sectionMap["description"]),
		)
		htmlBody := firstNonEmpty(
			coerceString(sectionMap["html"]),
			coerceString(sectionMap["contentHtml"]),
		)
		if htmlBody == "" && body != "" {
			htmlBody = "<p>" + html.EscapeString(body) + "</p>"
		}

		sections = append(sections, ArtifactSection{
			ID: firstNonEmpty(
				coerceString(sectionMap["id"]),
				coerceString(sectionMap["key"]),
				fmt.Sprintf("section-%d", index+1),
			),
			Title: firstNonEmpty(title, fmt.Sprintf("Section %d", index+1)),
			Body:  body,
			HTML:  htmlBody,
		})
	}
	return sections
}

func coerceArtifactSectionsFromTopLevel(raw map[string]any) []ArtifactSection {
	ignoredKeys := map[string]struct{}{
		"artifactType":  {},
		"title":         {},
		"summary":       {},
		"sections":      {},
		"highlights":    {},
		"openQuestions": {},
		"metadata":      {},
	}

	sections := make([]ArtifactSection, 0, len(raw))
	for key, value := range raw {
		if _, ignored := ignoredKeys[key]; ignored {
			continue
		}
		body := coerceString(value)
		if strings.TrimSpace(body) == "" {
			continue
		}
		sections = append(sections, ArtifactSection{
			ID:    slugify(key),
			Title: humanizeKey(key),
			Body:  body,
			HTML:  "<p>" + html.EscapeString(body) + "</p>",
		})
	}

	sort.SliceStable(sections, func(i, j int) bool {
		return sections[i].ID < sections[j].ID
	})
	return sections
}

func coerceStringSlice(value any) []string {
	switch typed := value.(type) {
	case nil:
		return []string{}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return []string{}
		}
		return []string{text}
	case []string:
		return uniqueStrings(typed)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := coerceString(item); text != "" {
				items = append(items, text)
			}
		}
		return uniqueStrings(items)
	default:
		if text := coerceString(typed); text != "" {
			return []string{text}
		}
		return []string{}
	}
}

func coerceStringMapAny(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return typed
}

func coerceString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64, float32, int, int32, int64, uint, uint32, uint64, bool:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := coerceString(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		for _, key := range []string{"text", "question", "title", "label", "name", "value", "body", "content", "description", "notes"} {
			if text := coerceString(typed[key]); text != "" {
				return text
			}
		}
		payload, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(payload))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func humanizeKey(value string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ")
	value = replacer.Replace(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var builder strings.Builder
	for index, r := range value {
		if index > 0 && unicode.IsUpper(r) && builder.Len() > 0 {
			builder.WriteRune(' ')
		}
		builder.WriteRune(r)
	}

	text := strings.TrimSpace(builder.String())
	if text == "" {
		return ""
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

func clipText(input string, limit int) string {
	input = strings.TrimSpace(input)
	if limit <= 0 || len(input) <= limit {
		return input
	}
	return strings.TrimSpace(input[:limit]) + "\n...(truncated)"
}

func findFirstValidJSONBlock(content string) (string, bool) {
	runes := []rune(content)
	for start := 0; start < len(runes); start++ {
		if runes[start] != '{' && runes[start] != '[' {
			continue
		}

		depth := 0
		inString := false
		escaped := false
		for end := start; end < len(runes); end++ {
			r := runes[end]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if r == '\\' {
					escaped = true
					continue
				}
				if r == '"' {
					inString = false
				}
				continue
			}

			switch r {
			case '"':
				inString = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					candidate := strings.TrimSpace(string(runes[start : end+1]))
					if json.Valid([]byte(candidate)) {
						return candidate, true
					}
					break
				}
			}
		}
	}
	return "", false
}

func sortBundles(bundles []ArtifactBundle) {
	sort.SliceStable(bundles, func(i, j int) bool {
		return artifactRank(bundles[i].Artifact.Type) < artifactRank(bundles[j].Artifact.Type)
	})
	for index := range bundles {
		sort.SliceStable(bundles[index].Versions, func(i, j int) bool {
			return bundles[index].Versions[i].VersionNumber > bundles[index].Versions[j].VersionNumber
		})
	}
}

func artifactRank(artifactType ArtifactType) int {
	switch artifactType {
	case ArtifactTypePRD:
		return 1
	case ArtifactTypeFunctionalSpec:
		return 2
	case ArtifactTypeTechnicalSpec:
		return 3
	case ArtifactTypePrototype:
		return 4
	default:
		return 99
	}
}

func downstreamArtifacts(source ArtifactType) []ArtifactType {
	switch source {
	case ArtifactTypePRD:
		return []ArtifactType{ArtifactTypeFunctionalSpec, ArtifactTypeTechnicalSpec, ArtifactTypePrototype}
	case ArtifactTypeFunctionalSpec:
		return []ArtifactType{ArtifactTypeTechnicalSpec, ArtifactTypePrototype}
	case ArtifactTypeTechnicalSpec:
		return []ArtifactType{ArtifactTypePrototype}
	default:
		return nil
	}
}

func artifactDisplayTitle(projectName string, artifactType ArtifactType) string {
	switch artifactType {
	case ArtifactTypePRD:
		return projectName + " PRD"
	case ArtifactTypeFunctionalSpec:
		return projectName + " 功能规格说明"
	case ArtifactTypeTechnicalSpec:
		return projectName + " 研发实现说明"
	case ArtifactTypePrototype:
		return projectName + " UISchema"
	default:
		return projectName + " 文档"
	}
}

func normalizeAPIFormat(value AIAPIFormat) AIAPIFormat {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case "chat", "chat-completions", string(AIAPIFormatChatCompletions):
		return AIAPIFormatChatCompletions
	case "response", string(AIAPIFormatResponses):
		return AIAPIFormatResponses
	default:
		return ""
	}
}
