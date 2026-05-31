package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SourceMaterialType string

const (
	SourceMaterialTypeFreeText     SourceMaterialType = "free_text"
	SourceMaterialTypePRDUpload    SourceMaterialType = "prd_upload"
	SourceMaterialTypeMeetingNotes SourceMaterialType = "meeting_notes"
	SourceMaterialTypeChatLog      SourceMaterialType = "chat_log"
	SourceMaterialTypeBidDoc       SourceMaterialType = "bid_doc"
	SourceMaterialTypeOtherFile    SourceMaterialType = "other_file"
)

type SourceMaterial struct {
	ID             string             `json:"id"`
	ProjectID      string             `json:"projectId"`
	SourceType     SourceMaterialType `json:"sourceType"`
	Title          string             `json:"title"`
	RawText        string             `json:"rawText"`
	FileName       string             `json:"fileName,omitempty"`
	MimeType       string             `json:"mimeType,omitempty"`
	FilePath       string             `json:"filePath,omitempty"`
	ParsedContent  map[string]any     `json:"parsedContent"`
	ExtractionMeta map[string]any     `json:"extractionMeta"`
	IsPrimary      bool               `json:"isPrimary"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

type RequirementIntake struct {
	ID                string         `json:"id"`
	ProjectID         string         `json:"projectId"`
	Title             string         `json:"title"`
	Summary           string         `json:"summary"`
	SourceMaterialIDs []string       `json:"sourceMaterialIds"`
	NormalizedInput   map[string]any `json:"normalizedInput"`
	DomainGuess       string         `json:"domainGuess"`
	ReadinessScore    int            `json:"readinessScore"`
	Status            string         `json:"status"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

type ConsensusBriefData struct {
	ProblemStatement string   `json:"problem_statement"`
	TargetUsers      []string `json:"target_users"`
	BusinessGoals    []string `json:"business_goals"`
	SuccessMetrics   []string `json:"success_metrics"`
	CoreScenarios    []string `json:"core_scenarios"`
	InScopeItems     []string `json:"in_scope_items"`
	OutOfScopeItems  []string `json:"out_of_scope_items"`
	Risks            []string `json:"risks"`
	Dependencies     []string `json:"dependencies"`
	Assumptions      []string `json:"assumptions"`
}

type ConsensusBrief struct {
	ID                  string             `json:"id"`
	ProjectID           string             `json:"projectId"`
	RequirementIntakeID string             `json:"requirementIntakeId"`
	VersionNumber       int                `json:"versionNumber"`
	Status              VersionStatus      `json:"status"`
	Structured          ConsensusBriefData `json:"structured"`
	RenderedMarkdown    string             `json:"renderedMarkdown"`
	QualityReport       map[string]any     `json:"qualityReport"`
	CreatedAt           time.Time          `json:"createdAt"`
	UpdatedAt           time.Time          `json:"updatedAt"`
}

type RequirementProblemDefinition struct {
	Summary string `json:"summary"`
	Problem string `json:"problem"`
	Outcome string `json:"outcome"`
}

type RequirementActor struct {
	Name             string   `json:"name"`
	Responsibilities []string `json:"responsibilities"`
	PainPoints       []string `json:"painPoints"`
}

type RequirementFlow struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
}

type RequirementScope struct {
	In  []string `json:"in"`
	Out []string `json:"out"`
}

type RequirementEntity struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Fields      []string `json:"fields"`
}

type RequirementRule struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RequirementModel struct {
	ID                 string                       `json:"id"`
	ProjectID          string                       `json:"projectId"`
	ConsensusBriefID   string                       `json:"consensusBriefId"`
	VersionNumber      int                          `json:"versionNumber"`
	Status             VersionStatus                `json:"status"`
	ProblemDefinition  RequirementProblemDefinition `json:"problem_definition"`
	Actors             []RequirementActor           `json:"actors"`
	Goals              []string                     `json:"goals"`
	Flows              []RequirementFlow            `json:"flows"`
	Scope              RequirementScope             `json:"scope"`
	Entities           []RequirementEntity          `json:"entities"`
	Rules              []RequirementRule            `json:"rules"`
	AcceptanceCriteria []string                     `json:"acceptance_criteria"`
	Constraints        []string                     `json:"constraints"`
	Traceability       map[string]any               `json:"traceability"`
	CreatedAt          time.Time                    `json:"createdAt"`
	UpdatedAt          time.Time                    `json:"updatedAt"`
}

type SaveSourceMaterialInput struct {
	ID             string             `json:"id,omitempty"`
	ProjectID      string             `json:"projectId"`
	SourceType     SourceMaterialType `json:"sourceType"`
	Title          string             `json:"title"`
	RawText        string             `json:"rawText"`
	FileName       string             `json:"fileName,omitempty"`
	MimeType       string             `json:"mimeType,omitempty"`
	FilePath       string             `json:"filePath,omitempty"`
	ParsedContent  map[string]any     `json:"parsedContent,omitempty"`
	ExtractionMeta map[string]any     `json:"extractionMeta,omitempty"`
	IsPrimary      bool               `json:"isPrimary"`
}

type ImportSourceMaterialFromFileInput struct {
	ProjectID  string             `json:"projectId"`
	SourceType SourceMaterialType `json:"sourceType"`
	Title      string             `json:"title,omitempty"`
	FilePath   string             `json:"filePath,omitempty"`
	IsPrimary  bool               `json:"isPrimary"`
}

type BuildRequirementIntakeInput struct {
	ProjectID string `json:"projectId"`
	Title     string `json:"title,omitempty"`
}

func normalizeSourceMaterial(material SourceMaterial) SourceMaterial {
	material.ParsedContent = ensureMap(material.ParsedContent)
	material.ExtractionMeta = ensureMap(material.ExtractionMeta)
	if material.SourceType == "" {
		material.SourceType = SourceMaterialTypeOtherFile
	}
	return material
}

func normalizeSourceMaterials(materials []SourceMaterial) []SourceMaterial {
	materials = ensureSlice(materials)
	for index := range materials {
		materials[index] = normalizeSourceMaterial(materials[index])
	}
	return materials
}

func normalizeRequirementIntake(intake RequirementIntake) RequirementIntake {
	intake.SourceMaterialIDs = ensureSlice(intake.SourceMaterialIDs)
	intake.NormalizedInput = ensureMap(intake.NormalizedInput)
	return intake
}

func normalizeConsensusBriefData(data ConsensusBriefData) ConsensusBriefData {
	data.TargetUsers = ensureSlice(data.TargetUsers)
	data.BusinessGoals = ensureSlice(data.BusinessGoals)
	data.SuccessMetrics = ensureSlice(data.SuccessMetrics)
	data.CoreScenarios = ensureSlice(data.CoreScenarios)
	data.InScopeItems = ensureSlice(data.InScopeItems)
	data.OutOfScopeItems = ensureSlice(data.OutOfScopeItems)
	data.Risks = ensureSlice(data.Risks)
	data.Dependencies = ensureSlice(data.Dependencies)
	data.Assumptions = ensureSlice(data.Assumptions)
	return data
}

func normalizeConsensusBrief(brief ConsensusBrief) ConsensusBrief {
	brief.Structured = normalizeConsensusBriefData(brief.Structured)
	brief.QualityReport = ensureMap(brief.QualityReport)
	return brief
}

func normalizeRequirementModel(model RequirementModel) RequirementModel {
	model.Actors = ensureSlice(model.Actors)
	for index := range model.Actors {
		model.Actors[index].Responsibilities = ensureSlice(model.Actors[index].Responsibilities)
		model.Actors[index].PainPoints = ensureSlice(model.Actors[index].PainPoints)
	}
	model.Goals = ensureSlice(model.Goals)
	model.Flows = ensureSlice(model.Flows)
	for index := range model.Flows {
		model.Flows[index].Steps = ensureSlice(model.Flows[index].Steps)
	}
	model.Scope.In = ensureSlice(model.Scope.In)
	model.Scope.Out = ensureSlice(model.Scope.Out)
	model.Entities = ensureSlice(model.Entities)
	for index := range model.Entities {
		model.Entities[index].Fields = ensureSlice(model.Entities[index].Fields)
	}
	model.Rules = ensureSlice(model.Rules)
	model.AcceptanceCriteria = ensureSlice(model.AcceptanceCriteria)
	model.Constraints = ensureSlice(model.Constraints)
	model.Traceability = ensureMap(model.Traceability)
	return model
}

func (data ConsensusBriefData) Validate() error {
	if strings.TrimSpace(data.ProblemStatement) == "" {
		return errors.New("consensus brief requires problem_statement")
	}
	if len(data.BusinessGoals) == 0 {
		return errors.New("consensus brief requires at least one business goal")
	}
	if len(data.InScopeItems) == 0 {
		return errors.New("consensus brief requires in_scope_items")
	}
	return nil
}

func (model RequirementModel) Validate() error {
	problem := strings.TrimSpace(model.ProblemDefinition.Summary)
	if problem == "" {
		problem = strings.TrimSpace(model.ProblemDefinition.Problem)
	}
	if problem == "" {
		return errors.New("requirement model requires problem_definition")
	}
	if len(model.Goals) == 0 {
		return errors.New("requirement model requires goals")
	}
	if len(model.Actors) == 0 {
		return errors.New("requirement model requires actors")
	}
	if len(model.Scope.In) == 0 {
		return errors.New("requirement model requires in-scope items")
	}
	return nil
}

func parseConsensusBriefJSON(payload string) (ConsensusBriefData, error) {
	var data ConsensusBriefData
	if strings.TrimSpace(payload) == "" {
		return data, errors.New("empty consensus brief payload")
	}
	if err := json.Unmarshal([]byte(payload), &data); err == nil {
		data = normalizeConsensusBriefData(data)
		if err := data.Validate(); err == nil {
			return data, nil
		}
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return data, err
	}
	data = normalizeConsensusBriefData(coerceConsensusBriefData(raw))
	return data, data.Validate()
}

func parseRequirementModelJSON(payload string) (RequirementModel, error) {
	var model RequirementModel
	if strings.TrimSpace(payload) == "" {
		return model, errors.New("empty requirement model payload")
	}
	if err := json.Unmarshal([]byte(payload), &model); err == nil {
		model = normalizeRequirementModel(model)
		if err := model.Validate(); err == nil {
			return model, nil
		}
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return model, err
	}
	model = normalizeRequirementModel(coerceRequirementModel(raw))
	return model, model.Validate()
}

func renderConsensusBriefMarkdown(data ConsensusBriefData) string {
	data = normalizeConsensusBriefData(data)
	sections := []string{
		"# 需求共识稿",
		"## 问题定义\n" + strings.TrimSpace(data.ProblemStatement),
		"## 目标用户\n- " + strings.Join(data.TargetUsers, "\n- "),
		"## 业务目标\n- " + strings.Join(data.BusinessGoals, "\n- "),
		"## 成功指标\n- " + strings.Join(data.SuccessMetrics, "\n- "),
		"## 核心场景\n- " + strings.Join(data.CoreScenarios, "\n- "),
		"## 范围内\n- " + strings.Join(data.InScopeItems, "\n- "),
		"## 范围外\n- " + strings.Join(data.OutOfScopeItems, "\n- "),
		"## 风险与依赖\n- " + strings.Join(uniqueStrings(append(data.Risks, data.Dependencies...)), "\n- "),
	}
	if len(data.Assumptions) > 0 {
		sections = append(sections, "## 假设\n- "+strings.Join(data.Assumptions, "\n- "))
	}
	return strings.Join(sections, "\n\n")
}

func buildConsensusQualityReport(data ConsensusBriefData) map[string]any {
	data = normalizeConsensusBriefData(data)
	missing := make([]string, 0)
	if strings.TrimSpace(data.ProblemStatement) == "" {
		missing = append(missing, "problem_statement")
	}
	if len(data.TargetUsers) == 0 {
		missing = append(missing, "target_users")
	}
	if len(data.BusinessGoals) == 0 {
		missing = append(missing, "business_goals")
	}
	if len(data.SuccessMetrics) == 0 {
		missing = append(missing, "success_metrics")
	}
	if len(data.InScopeItems) == 0 {
		missing = append(missing, "in_scope_items")
	}
	score := 100 - (len(missing) * 12)
	if score < 0 {
		score = 0
	}
	return map[string]any{
		"score":         score,
		"missingFields": missing,
		"hasRisks":      len(data.Risks) > 0,
		"hasDeps":       len(data.Dependencies) > 0,
	}
}

func coerceConsensusBriefData(raw map[string]any) ConsensusBriefData {
	return ConsensusBriefData{
		ProblemStatement: firstNonEmpty(
			coerceString(firstNonNil(raw["problem_statement"], raw["problemStatement"], raw["problem"], raw["summary"])),
			coerceString(firstNonNil(raw["problem_definition"], raw["problemDefinition"])),
		),
		TargetUsers:     coerceStringSlice(firstNonNil(raw["target_users"], raw["targetUsers"], raw["users"], raw["actors"])),
		BusinessGoals:   coerceStringSlice(firstNonNil(raw["business_goals"], raw["businessGoals"], raw["goals"])),
		SuccessMetrics:  coerceStringSlice(firstNonNil(raw["success_metrics"], raw["successMetrics"], raw["metrics"])),
		CoreScenarios:   coerceStringSlice(firstNonNil(raw["core_scenarios"], raw["coreScenarios"], raw["scenarios"])),
		InScopeItems:    coerceStringSlice(firstNonNil(raw["in_scope_items"], raw["inScopeItems"], raw["in_scope"], raw["scope_in"])),
		OutOfScopeItems: coerceStringSlice(firstNonNil(raw["out_of_scope_items"], raw["outOfScopeItems"], raw["out_of_scope"], raw["scope_out"])),
		Risks:           coerceStringSlice(raw["risks"]),
		Dependencies:    coerceStringSlice(firstNonNil(raw["dependencies"], raw["deps"])),
		Assumptions:     coerceStringSlice(raw["assumptions"]),
	}
}

func coerceRequirementModel(raw map[string]any) RequirementModel {
	return RequirementModel{
		ProblemDefinition:  coerceRequirementProblemDefinition(firstNonNil(raw["problem_definition"], raw["problemDefinition"])),
		Actors:             coerceRequirementActors(firstNonNil(raw["actors"], raw["roles"], raw["users"])),
		Goals:              coerceStringSlice(firstNonNil(raw["goals"], raw["business_goals"], raw["businessGoals"])),
		Flows:              coerceRequirementFlows(firstNonNil(raw["flows"], raw["journeys"], raw["core_flows"])),
		Scope:              coerceRequirementScope(firstNonNil(raw["scope"], raw["boundaries"])),
		Entities:           coerceRequirementEntities(firstNonNil(raw["entities"], raw["domain_entities"], raw["domainEntities"])),
		Rules:              coerceRequirementRules(firstNonNil(raw["rules"], raw["business_rules"], raw["businessRules"])),
		AcceptanceCriteria: coerceStringSlice(firstNonNil(raw["acceptance_criteria"], raw["acceptanceCriteria"], raw["acceptance"])),
		Constraints:        coerceStringSlice(firstNonNil(raw["constraints"], raw["technical_constraints"], raw["technicalConstraints"])),
		Traceability:       coerceStringMapAny(raw["traceability"]),
	}
}

func coerceRequirementProblemDefinition(value any) RequirementProblemDefinition {
	if typed, ok := value.(map[string]any); ok {
		return RequirementProblemDefinition{
			Summary: coerceString(firstNonNil(typed["summary"], typed["statement"])),
			Problem: coerceString(firstNonNil(typed["problem"], typed["detail"])),
			Outcome: coerceString(firstNonNil(typed["outcome"], typed["goal"])),
		}
	}
	text := coerceString(value)
	return RequirementProblemDefinition{
		Summary: text,
		Problem: text,
	}
}

func coerceRequirementActors(value any) []RequirementActor {
	entries := coerceKeyedEntries(value)
	actors := make([]RequirementActor, 0, len(entries))
	for index, entry := range entries {
		if typed, ok := entry.Value.(map[string]any); ok {
			actors = append(actors, RequirementActor{
				Name: firstNonEmpty(
					coerceString(firstNonNil(typed["name"], typed["title"], typed["role"])),
					entry.Key,
					fmt.Sprintf("actor-%d", index+1),
				),
				Responsibilities: coerceStringSlice(firstNonNil(typed["responsibilities"], typed["goals"])),
				PainPoints:       coerceStringSlice(firstNonNil(typed["pain_points"], typed["painPoints"], typed["problems"])),
			})
			continue
		}
		name := coerceString(entry.Value)
		if name == "" {
			continue
		}
		actors = append(actors, RequirementActor{Name: name})
	}
	return actors
}

func coerceRequirementFlows(value any) []RequirementFlow {
	entries := coerceKeyedEntries(value)
	flows := make([]RequirementFlow, 0, len(entries))
	for index, entry := range entries {
		if typed, ok := entry.Value.(map[string]any); ok {
			flows = append(flows, RequirementFlow{
				Name: firstNonEmpty(
					coerceString(firstNonNil(typed["name"], typed["title"])),
					entry.Key,
					fmt.Sprintf("flow-%d", index+1),
				),
				Steps: coerceStringSlice(firstNonNil(typed["steps"], typed["nodes"])),
			})
			continue
		}
		name := coerceString(entry.Value)
		if name == "" {
			continue
		}
		flows = append(flows, RequirementFlow{Name: name})
	}
	return flows
}

func coerceRequirementScope(value any) RequirementScope {
	if typed, ok := value.(map[string]any); ok {
		return RequirementScope{
			In:  coerceStringSlice(firstNonNil(typed["in"], typed["in_scope"], typed["inScope"], typed["scope_in"])),
			Out: coerceStringSlice(firstNonNil(typed["out"], typed["out_scope"], typed["outScope"], typed["scope_out"])),
		}
	}
	return RequirementScope{
		In:  coerceStringSlice(value),
		Out: []string{},
	}
}

func coerceRequirementEntities(value any) []RequirementEntity {
	entries := coerceKeyedEntries(value)
	entities := make([]RequirementEntity, 0, len(entries))
	for index, entry := range entries {
		if typed, ok := entry.Value.(map[string]any); ok {
			entities = append(entities, RequirementEntity{
				Name: firstNonEmpty(
					coerceString(firstNonNil(typed["name"], typed["title"])),
					entry.Key,
					fmt.Sprintf("entity-%d", index+1),
				),
				Description: coerceString(firstNonNil(typed["description"], typed["summary"])),
				Fields:      coerceStringSlice(firstNonNil(typed["fields"], typed["attributes"])),
			})
			continue
		}
		name := coerceString(entry.Value)
		if name == "" {
			continue
		}
		entities = append(entities, RequirementEntity{Name: name})
	}
	return entities
}

func coerceRequirementRules(value any) []RequirementRule {
	entries := coerceKeyedEntries(value)
	rules := make([]RequirementRule, 0, len(entries))
	for index, entry := range entries {
		if typed, ok := entry.Value.(map[string]any); ok {
			rules = append(rules, RequirementRule{
				Name: firstNonEmpty(
					coerceString(firstNonNil(typed["name"], typed["title"])),
					entry.Key,
					fmt.Sprintf("rule-%d", index+1),
				),
				Description: firstNonEmpty(
					coerceString(firstNonNil(typed["description"], typed["body"], typed["content"])),
					coerceString(typed["name"]),
				),
			})
			continue
		}
		text := coerceString(entry.Value)
		if text == "" {
			continue
		}
		rules = append(rules, RequirementRule{
			Name:        fmt.Sprintf("rule-%d", index+1),
			Description: text,
		})
	}
	return rules
}
