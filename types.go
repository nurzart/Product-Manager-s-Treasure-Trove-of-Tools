package main

import "time"

type ArtifactType string

const (
	ArtifactTypePRD            ArtifactType = "PRD"
	ArtifactTypeFunctionalSpec ArtifactType = "FunctionalSpec"
	ArtifactTypeTechnicalSpec  ArtifactType = "TechnicalSpec"
	ArtifactTypePrototype      ArtifactType = "PrototypeSource"
)

type VersionStatus string

const (
	VersionStatusDraft     VersionStatus = "draft"
	VersionStatusGenerated VersionStatus = "generated"
	VersionStatusReviewed  VersionStatus = "reviewed"
	VersionStatusApproved  VersionStatus = "approved"
	VersionStatusStale     VersionStatus = "stale"
)

type SyncStatus string

const (
	SyncStatusLocal   SyncStatus = "local"
	SyncStatusPending SyncStatus = "pending"
	SyncStatusSynced  SyncStatus = "synced"
)

type AIAPIFormat string

const (
	AIAPIFormatResponses       AIAPIFormat = "responses"
	AIAPIFormatChatCompletions AIAPIFormat = "chat_completions"
)

type Project struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	OwnerID     string     `json:"ownerId"`
	RemoteID    string     `json:"remoteId,omitempty"`
	SyncStatus  SyncStatus `json:"syncStatus"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type RequirementSummary struct {
	ProductName   string   `json:"productName"`
	TargetUsers   []string `json:"targetUsers"`
	BusinessGoals []string `json:"businessGoals"`
	CoreScenarios []string `json:"coreScenarios"`
	Constraints   []string `json:"constraints"`
	OpenQuestions []string `json:"openQuestions"`
}

type ClarificationQuestion struct {
	ID        string `json:"id"`
	Question  string `json:"question"`
	Rationale string `json:"rationale"`
	Priority  string `json:"priority"`
	Answer    string `json:"answer,omitempty"`
}

type ClarificationAnswer struct {
	QuestionID string `json:"questionId"`
	Answer     string `json:"answer"`
}

type RequirementInput struct {
	ID             string                  `json:"id"`
	ProjectID      string                  `json:"projectId"`
	RawInput       string                  `json:"rawInput"`
	Summary        RequirementSummary      `json:"summary"`
	Clarifications []ClarificationQuestion `json:"clarifications"`
	Answers        []ClarificationAnswer   `json:"answers"`
	CreatedAt      time.Time               `json:"createdAt"`
	UpdatedAt      time.Time               `json:"updatedAt"`
}

type Artifact struct {
	ID         string       `json:"id"`
	ProjectID  string       `json:"projectId"`
	Type       ArtifactType `json:"type"`
	Title      string       `json:"title"`
	OwnerID    string       `json:"ownerId"`
	RemoteID   string       `json:"remoteId,omitempty"`
	SyncStatus SyncStatus   `json:"syncStatus"`
	CreatedAt  time.Time    `json:"createdAt"`
	UpdatedAt  time.Time    `json:"updatedAt"`
}

type ArtifactVersion struct {
	ID                string        `json:"id"`
	ArtifactID        string        `json:"artifactId"`
	ProjectID         string        `json:"projectId"`
	Type              ArtifactType  `json:"type"`
	VersionNumber     int           `json:"versionNumber"`
	SourceVersionID   string        `json:"sourceVersionId,omitempty"`
	Status            VersionStatus `json:"status"`
	StructuredJSON    string        `json:"structuredJson"`
	RenderedMarkdown  string        `json:"renderedMarkdown"`
	PlainTextSnapshot string        `json:"plainTextSnapshot"`
	GenerationMeta    string        `json:"generationMeta"`
	CreatedAt         time.Time     `json:"createdAt"`
	UpdatedAt         time.Time     `json:"updatedAt"`
}

type ArtifactBundle struct {
	Artifact Artifact          `json:"artifact"`
	Versions []ArtifactVersion `json:"versions"`
}

type WorkflowRun struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"projectId"`
	RunType      string    `json:"runType"`
	Status       string    `json:"status"`
	InputJSON    string    `json:"inputJson"`
	OutputJSON   string    `json:"outputJson"`
	ErrorMessage string    `json:"errorMessage"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type ProviderConfig struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	ProviderType string      `json:"providerType"`
	BaseURL      string      `json:"baseUrl"`
	APIKey       string      `json:"apiKey"`
	Model        string      `json:"model"`
	APIFormat    AIAPIFormat `json:"apiFormat"`
	Enabled      bool        `json:"enabled"`
	UseMock      bool        `json:"useMock"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

type ProviderConnectionResult struct {
	OK              bool        `json:"ok"`
	ContentReady    bool        `json:"contentReady"`
	Descriptor      string      `json:"descriptor"`
	BaseURL         string      `json:"baseUrl"`
	ResolvedBaseURL string      `json:"resolvedBaseUrl"`
	APIFormat       AIAPIFormat `json:"apiFormat"`
	Model           string      `json:"model"`
	ModelsCount     int         `json:"modelsCount"`
	LatencyMs       int64       `json:"latencyMs"`
	Message         string      `json:"message"`
	ContentMessage  string      `json:"contentMessage,omitempty"`
	Diagnostic      string      `json:"diagnostic,omitempty"`
}

type ArtifactSection struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	HTML        string `json:"html"`
	Description string `json:"description,omitempty"`
}

type ArtifactDocument struct {
	ArtifactType  ArtifactType      `json:"artifactType"`
	Title         string            `json:"title"`
	Summary       string            `json:"summary"`
	Sections      []ArtifactSection `json:"sections"`
	Highlights    []string          `json:"highlights,omitempty"`
	OpenQuestions []string          `json:"openQuestions,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty"`
}

type UISchema struct {
	AppMeta       UIAppMeta        `json:"app_meta"`
	Routes        []UIRoute        `json:"routes"`
	Pages         []UIPage         `json:"pages"`
	Components    []UIComponent    `json:"components"`
	Forms         []UIForm         `json:"forms"`
	Tables        []UITable        `json:"tables"`
	Actions       []UIAction       `json:"actions"`
	MockData      map[string]any   `json:"mock_data"`
	StateVariants []UIStateVariant `json:"state_variants"`
	NavigationMap []UINavigation   `json:"navigation_map"`
}

type UIAppMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UIRoute struct {
	Path   string `json:"path"`
	PageID string `json:"page_id"`
	Label  string `json:"label"`
}

type UIPage struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Type          string   `json:"type"`
	Description   string   `json:"description"`
	Layout        string   `json:"layout"`
	Components    []string `json:"components"`
	States        []string `json:"states"`
	PrimaryCTAs   []string `json:"primary_ctas"`
	SecondaryCTAs []string `json:"secondary_ctas"`
}

type UIComponent struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	PageID      string         `json:"page_id"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Props       map[string]any `json:"props"`
}

type UIForm struct {
	ID        string    `json:"id"`
	PageID    string    `json:"page_id"`
	Title     string    `json:"title"`
	Fields    []UIField `json:"fields"`
	SubmitCTA string    `json:"submit_cta"`
	States    []string  `json:"states"`
}

type UITable struct {
	ID      string   `json:"id"`
	PageID  string   `json:"page_id"`
	Title   string   `json:"title"`
	Columns []string `json:"columns"`
}

type UIAction struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	SourcePageID string `json:"source_page_id"`
	TargetPageID string `json:"target_page_id,omitempty"`
	Description  string `json:"description"`
}

type UIStateVariant struct {
	PageID string `json:"page_id"`
	State  string `json:"state"`
	Notes  string `json:"notes"`
}

type UINavigation struct {
	FromPageID string `json:"from_page_id"`
	ToPageID   string `json:"to_page_id"`
	Trigger    string `json:"trigger"`
}

type UIField struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Placeholder string   `json:"placeholder"`
	Required    bool     `json:"required"`
	Options     []string `json:"options,omitempty"`
}

type GeneratedFile struct {
	Path     string `json:"path"`
	Contents string `json:"contents"`
}

type PrototypeDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
	PageID   string `json:"pageId,omitempty"`
}

type PrototypeRepairReport struct {
	Applied           bool                  `json:"applied"`
	Strategy          string                `json:"strategy"`
	Summary           string                `json:"summary"`
	Issue             string                `json:"issue"`
	ExpectedBehavior  string                `json:"expectedBehavior,omitempty"`
	OptimizationNotes string                `json:"optimizationNotes,omitempty"`
	TargetPageID      string                `json:"targetPageId,omitempty"`
	Diagnostics       []PrototypeDiagnostic `json:"diagnostics"`
}

type PrototypeBundle struct {
	ProjectID     string                 `json:"projectId"`
	ProjectName   string                 `json:"projectName"`
	EntryFile     string                 `json:"entryFile"`
	ExportPath    string                 `json:"exportPath,omitempty"`
	GeneratedAt   time.Time              `json:"generatedAt"`
	UISchema      UISchema               `json:"uiSchema"`
	RenderMode    string                 `json:"renderMode,omitempty"`
	DesignSummary string                 `json:"designSummary,omitempty"`
	PreviewHTML   string                 `json:"previewHtml,omitempty"`
	Diagnostics   []PrototypeDiagnostic  `json:"diagnostics"`
	RepairReport  *PrototypeRepairReport `json:"repairReport,omitempty"`
	Files         []GeneratedFile        `json:"files"`
}

type WorkspaceSnapshot struct {
	Project           Project            `json:"project"`
	RequirementInput  *RequirementInput  `json:"requirementInput"`
	SourceMaterials   []SourceMaterial   `json:"sourceMaterials"`
	RequirementIntake *RequirementIntake `json:"requirementIntake,omitempty"`
	ConsensusBrief    *ConsensusBrief    `json:"consensusBrief,omitempty"`
	RequirementModel  *RequirementModel  `json:"requirementModel,omitempty"`
	Artifacts         []ArtifactBundle   `json:"artifacts"`
	WorkflowRuns      []WorkflowRun      `json:"workflowRuns"`
}

type CreateProjectInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SaveRequirementInputInput struct {
	ProjectID string `json:"projectId"`
	RawInput  string `json:"rawInput"`
}

type PRDBaselineSectionInput struct {
	Key     string `json:"key"`
	Content string `json:"content"`
}

type SavePRDBaselineInput struct {
	SourceMaterialID string                    `json:"sourceMaterialId"`
	Sections         []PRDBaselineSectionInput `json:"sections"`
}

type AnswerClarificationsInput struct {
	ProjectID string                `json:"projectId"`
	Answers   []ClarificationAnswer `json:"answers"`
}

type GenerateArtifactInput struct {
	ProjectID    string       `json:"projectId"`
	ArtifactType ArtifactType `json:"artifactType"`
}

type UpdateArtifactSectionInput struct {
	VersionID   string `json:"versionId"`
	SectionID   string `json:"sectionId"`
	Content     string `json:"content"`
	ContentHTML string `json:"contentHtml"`
}

type ApproveArtifactVersionInput struct {
	VersionID string `json:"versionId"`
}

type RegenerateDownstreamArtifactsInput struct {
	ProjectID          string       `json:"projectId"`
	SourceArtifactType ArtifactType `json:"sourceArtifactType"`
}

type ExportPrototypeInput struct {
	ProjectID string `json:"projectId"`
	OutputDir string `json:"outputDir"`
}

type PrototypeRepairInput struct {
	ProjectID         string `json:"projectId"`
	PageID            string `json:"pageId,omitempty"`
	CurrentIssue      string `json:"currentIssue"`
	ExpectedBehavior  string `json:"expectedBehavior,omitempty"`
	OptimizationNotes string `json:"optimizationNotes,omitempty"`
}
