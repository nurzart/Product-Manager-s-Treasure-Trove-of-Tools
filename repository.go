package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(databasePath string) (*Repository, error) {
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	repo := &Repository{db: db}
	if err := repo.migrate(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *Repository) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			remote_id TEXT NOT NULL DEFAULT '',
			sync_status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS requirement_inputs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL UNIQUE,
			raw_input TEXT NOT NULL,
			summary_json TEXT NOT NULL,
			clarifications_json TEXT NOT NULL,
			answers_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS source_materials (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			source_type TEXT NOT NULL,
			title TEXT NOT NULL,
			raw_text TEXT NOT NULL,
			file_name TEXT NOT NULL DEFAULT '',
			mime_type TEXT NOT NULL DEFAULT '',
			file_path TEXT NOT NULL DEFAULT '',
			parsed_content_json TEXT NOT NULL DEFAULT '{}',
			extraction_meta_json TEXT NOT NULL DEFAULT '{}',
			is_primary INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS requirement_intakes (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			title TEXT NOT NULL,
			summary TEXT NOT NULL,
			source_material_ids_json TEXT NOT NULL,
			normalized_input_json TEXT NOT NULL,
			domain_guess TEXT NOT NULL DEFAULT '',
			readiness_score INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS consensus_briefs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			requirement_intake_id TEXT NOT NULL DEFAULT '',
			version_number INTEGER NOT NULL,
			status TEXT NOT NULL,
			structured_json TEXT NOT NULL,
			rendered_markdown TEXT NOT NULL,
			quality_report_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS requirement_models (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			consensus_brief_id TEXT NOT NULL DEFAULT '',
			version_number INTEGER NOT NULL,
			status TEXT NOT NULL,
			problem_definition_json TEXT NOT NULL,
			actors_json TEXT NOT NULL,
			goals_json TEXT NOT NULL,
			flows_json TEXT NOT NULL,
			scope_json TEXT NOT NULL,
			entities_json TEXT NOT NULL,
			rules_json TEXT NOT NULL,
			acceptance_json TEXT NOT NULL,
			constraints_json TEXT NOT NULL,
			traceability_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			remote_id TEXT NOT NULL DEFAULT '',
			sync_status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(project_id, type),
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS artifact_versions (
			id TEXT PRIMARY KEY,
			artifact_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			type TEXT NOT NULL,
			version_number INTEGER NOT NULL,
			source_version_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			structured_json TEXT NOT NULL,
			rendered_markdown TEXT NOT NULL,
			plain_text_snapshot TEXT NOT NULL,
			generation_meta TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(artifact_id) REFERENCES artifacts(id) ON DELETE CASCADE,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_runs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			run_type TEXT NOT NULL,
			status TEXT NOT NULL,
			input_json TEXT NOT NULL,
			output_json TEXT NOT NULL,
			error_message TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS provider_configs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider_type TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			model TEXT NOT NULL,
			api_format TEXT NOT NULL DEFAULT 'responses',
			enabled INTEGER NOT NULL,
			use_mock INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := r.ensureColumn(ctx, "provider_configs", "api_format", "TEXT NOT NULL DEFAULT 'responses'"); err != nil {
		return err
	}
	return nil
}

func (r *Repository) ensureColumn(ctx context.Context, tableName, columnName, columnDefinition string) error {
	rows, err := r.db.QueryContext(ctx, "PRAGMA table_info("+tableName+")")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			dataType   string
			notNull    int
			defaultV   any
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultV, &primaryKey); err != nil {
			return err
		}
		if name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "ALTER TABLE "+tableName+" ADD COLUMN "+columnName+" "+columnDefinition)
	return err
}

func (r *Repository) CreateProject(ctx context.Context, input CreateProjectInput) (Project, error) {
	now := nowUTC()
	project := Project{
		ID:          uuid.NewString(),
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		OwnerID:     "local-user",
		SyncStatus:  SyncStatusLocal,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if project.Name == "" {
		return Project{}, errors.New("project name is required")
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO projects (id, name, description, owner_id, remote_id, sync_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		project.ID,
		project.Name,
		project.Description,
		project.OwnerID,
		project.RemoteID,
		project.SyncStatus,
		project.CreatedAt.Format(timeLayout),
		project.UpdatedAt.Format(timeLayout),
	)
	if err != nil {
		return Project{}, err
	}
	return project, nil
}

func (r *Repository) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, description, owner_id, remote_id, sync_status, created_at, updated_at FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := make([]Project, 0)
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return normalizeProjects(projects), rows.Err()
}

func (r *Repository) GetProject(ctx context.Context, projectID string) (Project, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, description, owner_id, remote_id, sync_status, created_at, updated_at FROM projects WHERE id = ?`, projectID)
	return scanProject(row)
}

func (r *Repository) SaveRequirementInput(ctx context.Context, input SaveRequirementInputInput) (RequirementInput, error) {
	existing, err := r.GetRequirementInputByProject(ctx, input.ProjectID)
	now := nowUTC()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RequirementInput{}, err
	}

	requirement := RequirementInput{
		ID:             uuid.NewString(),
		ProjectID:      input.ProjectID,
		RawInput:       strings.TrimSpace(input.RawInput),
		Summary:        RequirementSummary{},
		Clarifications: []ClarificationQuestion{},
		Answers:        []ClarificationAnswer{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if requirement.RawInput == "" {
		return RequirementInput{}, errors.New("requirement input cannot be empty")
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, err = r.db.ExecContext(
			ctx,
			`INSERT INTO requirement_inputs (id, project_id, raw_input, summary_json, clarifications_json, answers_json, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			requirement.ID,
			requirement.ProjectID,
			requirement.RawInput,
			mustJSON(requirement.Summary),
			mustJSON(requirement.Clarifications),
			mustJSON(requirement.Answers),
			requirement.CreatedAt.Format(timeLayout),
			requirement.UpdatedAt.Format(timeLayout),
		)
		if err != nil {
			return RequirementInput{}, err
		}
	} else {
		requirement.ID = existing.ID
		requirement.CreatedAt = existing.CreatedAt
		_, err = r.db.ExecContext(
			ctx,
			`UPDATE requirement_inputs
			 SET raw_input = ?, summary_json = ?, clarifications_json = ?, answers_json = ?, updated_at = ?
			 WHERE project_id = ?`,
			requirement.RawInput,
			mustJSON(requirement.Summary),
			mustJSON(requirement.Clarifications),
			mustJSON(requirement.Answers),
			requirement.UpdatedAt.Format(timeLayout),
			requirement.ProjectID,
		)
		if err != nil {
			return RequirementInput{}, err
		}
	}
	if err := r.touchProject(ctx, requirement.ProjectID, now); err != nil {
		return RequirementInput{}, err
	}
	return requirement, nil
}

func (r *Repository) UpdateRequirementInput(ctx context.Context, requirement RequirementInput) error {
	requirement.UpdatedAt = nowUTC()
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE requirement_inputs
		 SET raw_input = ?, summary_json = ?, clarifications_json = ?, answers_json = ?, updated_at = ?
		 WHERE project_id = ?`,
		requirement.RawInput,
		mustJSON(requirement.Summary),
		mustJSON(requirement.Clarifications),
		mustJSON(requirement.Answers),
		requirement.UpdatedAt.Format(timeLayout),
		requirement.ProjectID,
	)
	if err != nil {
		return err
	}
	return r.touchProject(ctx, requirement.ProjectID, requirement.UpdatedAt)
}

func (r *Repository) GetRequirementInputByProject(ctx context.Context, projectID string) (RequirementInput, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, raw_input, summary_json, clarifications_json, answers_json, created_at, updated_at
		 FROM requirement_inputs WHERE project_id = ?`,
		projectID,
	)
	var (
		requirement                                  RequirementInput
		summaryJSON, clarificationsJSON, answersJSON string
		createdAt, updatedAt                         string
	)
	if err := row.Scan(
		&requirement.ID,
		&requirement.ProjectID,
		&requirement.RawInput,
		&summaryJSON,
		&clarificationsJSON,
		&answersJSON,
		&createdAt,
		&updatedAt,
	); err != nil {
		return RequirementInput{}, err
	}
	if err := decodeJSON(summaryJSON, &requirement.Summary); err != nil {
		return RequirementInput{}, err
	}
	if err := decodeJSON(clarificationsJSON, &requirement.Clarifications); err != nil {
		return RequirementInput{}, err
	}
	if err := decodeJSON(answersJSON, &requirement.Answers); err != nil {
		return RequirementInput{}, err
	}
	var err error
	requirement.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return RequirementInput{}, err
	}
	requirement.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return RequirementInput{}, err
	}
	return normalizeRequirementInput(requirement), nil
}

func (r *Repository) EnsureArtifact(ctx context.Context, projectID string, artifactType ArtifactType, title string) (Artifact, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, type, title, owner_id, remote_id, sync_status, created_at, updated_at
		 FROM artifacts WHERE project_id = ? AND type = ?`,
		projectID,
		string(artifactType),
	)
	artifact, err := scanArtifact(row)
	if err == nil {
		if artifact.Title != title && strings.TrimSpace(title) != "" {
			now := nowUTC()
			if _, execErr := r.db.ExecContext(ctx, `UPDATE artifacts SET title = ?, updated_at = ? WHERE id = ?`, title, now.Format(timeLayout), artifact.ID); execErr == nil {
				artifact.Title = title
				artifact.UpdatedAt = now
			}
		}
		return artifact, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Artifact{}, err
	}

	now := nowUTC()
	artifact = Artifact{
		ID:         uuid.NewString(),
		ProjectID:  projectID,
		Type:       artifactType,
		Title:      title,
		OwnerID:    "local-user",
		SyncStatus: SyncStatusLocal,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO artifacts (id, project_id, type, title, owner_id, remote_id, sync_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		artifact.ID,
		artifact.ProjectID,
		string(artifact.Type),
		artifact.Title,
		artifact.OwnerID,
		artifact.RemoteID,
		artifact.SyncStatus,
		artifact.CreatedAt.Format(timeLayout),
		artifact.UpdatedAt.Format(timeLayout),
	)
	if err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func (r *Repository) CreateArtifactVersion(ctx context.Context, artifact Artifact, version ArtifactVersion) (ArtifactVersion, error) {
	row := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_number), 0) FROM artifact_versions WHERE artifact_id = ?`, artifact.ID)
	if err := row.Scan(&version.VersionNumber); err != nil {
		return ArtifactVersion{}, err
	}
	version.VersionNumber++
	now := nowUTC()
	version.ID = uuid.NewString()
	version.ArtifactID = artifact.ID
	version.ProjectID = artifact.ProjectID
	version.Type = artifact.Type
	version.CreatedAt = now
	version.UpdatedAt = now

	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO artifact_versions
		 (id, artifact_id, project_id, type, version_number, source_version_id, status, structured_json, rendered_markdown, plain_text_snapshot, generation_meta, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		version.ID,
		version.ArtifactID,
		version.ProjectID,
		string(version.Type),
		version.VersionNumber,
		version.SourceVersionID,
		string(version.Status),
		version.StructuredJSON,
		version.RenderedMarkdown,
		version.PlainTextSnapshot,
		version.GenerationMeta,
		version.CreatedAt.Format(timeLayout),
		version.UpdatedAt.Format(timeLayout),
	)
	if err != nil {
		return ArtifactVersion{}, err
	}
	if err := r.touchProject(ctx, artifact.ProjectID, now); err != nil {
		return ArtifactVersion{}, err
	}
	return version, nil
}

func (r *Repository) UpdateArtifactVersion(ctx context.Context, version ArtifactVersion) error {
	version.UpdatedAt = nowUTC()
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE artifact_versions
		 SET source_version_id = ?, status = ?, structured_json = ?, rendered_markdown = ?, plain_text_snapshot = ?, generation_meta = ?, updated_at = ?
		 WHERE id = ?`,
		version.SourceVersionID,
		string(version.Status),
		version.StructuredJSON,
		version.RenderedMarkdown,
		version.PlainTextSnapshot,
		version.GenerationMeta,
		version.UpdatedAt.Format(timeLayout),
		version.ID,
	)
	if err != nil {
		return err
	}
	return r.touchProject(ctx, version.ProjectID, version.UpdatedAt)
}

func (r *Repository) GetArtifactVersion(ctx context.Context, versionID string) (ArtifactVersion, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, artifact_id, project_id, type, version_number, source_version_id, status, structured_json, rendered_markdown, plain_text_snapshot, generation_meta, created_at, updated_at FROM artifact_versions WHERE id = ?`, versionID)
	return scanArtifactVersion(row)
}

func (r *Repository) GetLatestArtifactVersion(ctx context.Context, projectID string, artifactType ArtifactType) (ArtifactVersion, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT av.id, av.artifact_id, av.project_id, av.type, av.version_number, av.source_version_id, av.status, av.structured_json, av.rendered_markdown, av.plain_text_snapshot, av.generation_meta, av.created_at, av.updated_at
		 FROM artifact_versions av
		 INNER JOIN artifacts a ON a.id = av.artifact_id
		 WHERE av.project_id = ? AND a.type = ?
		 ORDER BY av.version_number DESC
		 LIMIT 1`,
		projectID,
		string(artifactType),
	)
	return scanArtifactVersion(row)
}

func (r *Repository) GetArtifactByType(ctx context.Context, projectID string, artifactType ArtifactType) (Artifact, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, project_id, type, title, owner_id, remote_id, sync_status, created_at, updated_at FROM artifacts WHERE project_id = ? AND type = ?`, projectID, string(artifactType))
	return scanArtifact(row)
}

func (r *Repository) ListArtifactBundles(ctx context.Context, projectID string) ([]ArtifactBundle, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, project_id, type, title, owner_id, remote_id, sync_status, created_at, updated_at FROM artifacts WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}

	artifacts := make([]Artifact, 0)
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	bundles := make([]ArtifactBundle, 0, len(artifacts))
	for _, artifact := range artifacts {
		versions, err := r.listArtifactVersions(ctx, artifact.ID)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, ArtifactBundle{Artifact: artifact, Versions: versions})
	}
	sortBundles(bundles)
	return normalizeArtifactBundles(bundles), nil
}

func (r *Repository) listArtifactVersions(ctx context.Context, artifactID string) ([]ArtifactVersion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, artifact_id, project_id, type, version_number, source_version_id, status, structured_json, rendered_markdown, plain_text_snapshot, generation_meta, created_at, updated_at FROM artifact_versions WHERE artifact_id = ? ORDER BY version_number DESC`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make([]ArtifactVersion, 0)
	for rows.Next() {
		version, err := scanArtifactVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return ensureSlice(versions), rows.Err()
}

func (r *Repository) MarkDownstreamArtifactsStale(ctx context.Context, projectID string, source ArtifactType) error {
	for _, artifactType := range downstreamArtifacts(source) {
		if err := r.markLatestArtifactVersionStatus(ctx, projectID, artifactType, VersionStatusStale); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (r *Repository) MarkAllArtifactsStale(ctx context.Context, projectID string) error {
	rows, err := r.db.QueryContext(ctx, `SELECT type FROM artifacts WHERE project_id = ?`, projectID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var types []ArtifactType
	for rows.Next() {
		var artifactType string
		if err := rows.Scan(&artifactType); err != nil {
			return err
		}
		types = append(types, ArtifactType(artifactType))
	}
	for _, artifactType := range types {
		if err := r.markLatestArtifactVersionStatus(ctx, projectID, artifactType, VersionStatusStale); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (r *Repository) markLatestArtifactVersionStatus(ctx context.Context, projectID string, artifactType ArtifactType, status VersionStatus) error {
	version, err := r.GetLatestArtifactVersion(ctx, projectID, artifactType)
	if err != nil {
		return err
	}
	version.Status = status
	meta := map[string]any{
		"marked_stale_at": nowUTC().Format(timeLayout),
	}
	if version.GenerationMeta != "" {
		meta["previous_meta"] = version.GenerationMeta
	}
	version.GenerationMeta = mustJSON(meta)
	return r.UpdateArtifactVersion(ctx, version)
}

func (r *Repository) CreateWorkflowRun(ctx context.Context, projectID, runType, inputJSON string) (WorkflowRun, error) {
	now := nowUTC()
	run := WorkflowRun{
		ID:         uuid.NewString(),
		ProjectID:  projectID,
		RunType:    runType,
		Status:     "running",
		InputJSON:  inputJSON,
		OutputJSON: "{}",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO workflow_runs (id, project_id, run_type, status, input_json, output_json, error_message, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		run.ProjectID,
		run.RunType,
		run.Status,
		run.InputJSON,
		run.OutputJSON,
		run.ErrorMessage,
		run.CreatedAt.Format(timeLayout),
		run.UpdatedAt.Format(timeLayout),
	)
	if err != nil {
		return WorkflowRun{}, err
	}
	return run, nil
}

func (r *Repository) FinishWorkflowRun(ctx context.Context, runID, status, outputJSON, errorMessage string) error {
	now := nowUTC()
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE workflow_runs SET status = ?, output_json = ?, error_message = ?, updated_at = ? WHERE id = ?`,
		status,
		outputJSON,
		errorMessage,
		now.Format(timeLayout),
		runID,
	)
	return err
}

func (r *Repository) ListWorkflowRuns(ctx context.Context, projectID string) ([]WorkflowRun, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, project_id, run_type, status, input_json, output_json, error_message, created_at, updated_at FROM workflow_runs WHERE project_id = ? ORDER BY created_at DESC LIMIT 30`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]WorkflowRun, 0)
	for rows.Next() {
		run, err := scanWorkflowRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return ensureSlice(runs), rows.Err()
}

func (r *Repository) GetProviderConfig(ctx context.Context) (ProviderConfig, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, provider_type, base_url, api_key, model, api_format, enabled, use_mock, created_at, updated_at FROM provider_configs ORDER BY updated_at DESC LIMIT 1`)
	return scanProviderConfig(row)
}

func (r *Repository) SaveProviderConfig(ctx context.Context, config ProviderConfig) (ProviderConfig, error) {
	now := nowUTC()
	if strings.TrimSpace(config.ID) == "" {
		config.ID = uuid.NewString()
		config.CreatedAt = now
	} else {
		current, err := r.GetProviderConfig(ctx)
		if err == nil {
			config.CreatedAt = current.CreatedAt
		} else {
			config.CreatedAt = now
		}
	}
	config.UpdatedAt = now
	if strings.TrimSpace(config.Name) == "" {
		config.Name = "Default Provider"
	}
	if strings.TrimSpace(config.ProviderType) == "" {
		config.ProviderType = "openai-compatible"
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	config.BaseURL = normalizeProviderBaseURL(config.BaseURL)
	if strings.TrimSpace(config.Model) == "" {
		config.Model = "gpt-4.1-mini"
	}
	if normalizeAPIFormat(config.APIFormat) == "" {
		config.APIFormat = AIAPIFormatResponses
	} else {
		config.APIFormat = normalizeAPIFormat(config.APIFormat)
	}

	_, err := r.db.ExecContext(ctx, `DELETE FROM provider_configs`)
	if err != nil {
		return ProviderConfig{}, err
	}
	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO provider_configs (id, name, provider_type, base_url, api_key, model, api_format, enabled, use_mock, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		config.ID,
		config.Name,
		config.ProviderType,
		config.BaseURL,
		config.APIKey,
		config.Model,
		string(config.APIFormat),
		boolToInt(config.Enabled),
		boolToInt(config.UseMock),
		config.CreatedAt.Format(timeLayout),
		config.UpdatedAt.Format(timeLayout),
	)
	if err != nil {
		return ProviderConfig{}, err
	}
	return config, nil
}

func (r *Repository) touchProject(ctx context.Context, projectID string, updatedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE projects SET updated_at = ? WHERE id = ?`, updatedAt.Format(timeLayout), projectID)
	return err
}

func scanProject(scanner interface{ Scan(dest ...any) error }) (Project, error) {
	var (
		project              Project
		syncStatus           string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&project.ID, &project.Name, &project.Description, &project.OwnerID, &project.RemoteID, &syncStatus, &createdAt, &updatedAt); err != nil {
		return Project{}, err
	}
	project.SyncStatus = SyncStatus(syncStatus)
	var err error
	project.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return Project{}, err
	}
	project.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return Project{}, err
	}
	return project, nil
}

func scanArtifact(scanner interface{ Scan(dest ...any) error }) (Artifact, error) {
	var (
		artifact             Artifact
		syncStatus           string
		typeValue            string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&artifact.ID, &artifact.ProjectID, &typeValue, &artifact.Title, &artifact.OwnerID, &artifact.RemoteID, &syncStatus, &createdAt, &updatedAt); err != nil {
		return Artifact{}, err
	}
	artifact.Type = ArtifactType(typeValue)
	artifact.SyncStatus = SyncStatus(syncStatus)
	var err error
	artifact.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return Artifact{}, err
	}
	artifact.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func scanArtifactVersion(scanner interface{ Scan(dest ...any) error }) (ArtifactVersion, error) {
	var (
		version              ArtifactVersion
		typeValue            string
		statusValue          string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(
		&version.ID,
		&version.ArtifactID,
		&version.ProjectID,
		&typeValue,
		&version.VersionNumber,
		&version.SourceVersionID,
		&statusValue,
		&version.StructuredJSON,
		&version.RenderedMarkdown,
		&version.PlainTextSnapshot,
		&version.GenerationMeta,
		&createdAt,
		&updatedAt,
	); err != nil {
		return ArtifactVersion{}, err
	}
	version.Type = ArtifactType(typeValue)
	version.Status = VersionStatus(statusValue)
	var err error
	version.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return ArtifactVersion{}, err
	}
	version.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return ArtifactVersion{}, err
	}
	return version, nil
}

func scanWorkflowRun(scanner interface{ Scan(dest ...any) error }) (WorkflowRun, error) {
	var run WorkflowRun
	var createdAt, updatedAt string
	if err := scanner.Scan(&run.ID, &run.ProjectID, &run.RunType, &run.Status, &run.InputJSON, &run.OutputJSON, &run.ErrorMessage, &createdAt, &updatedAt); err != nil {
		return WorkflowRun{}, err
	}
	var err error
	run.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return WorkflowRun{}, err
	}
	run.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return WorkflowRun{}, err
	}
	return run, nil
}

func scanProviderConfig(scanner interface{ Scan(dest ...any) error }) (ProviderConfig, error) {
	var (
		config               ProviderConfig
		apiFormat            string
		enabled, useMock     int
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&config.ID, &config.Name, &config.ProviderType, &config.BaseURL, &config.APIKey, &config.Model, &apiFormat, &enabled, &useMock, &createdAt, &updatedAt); err != nil {
		return ProviderConfig{}, err
	}
	config.APIFormat = normalizeAPIFormat(AIAPIFormat(apiFormat))
	config.Enabled = enabled == 1
	config.UseMock = useMock == 1
	var err error
	config.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return ProviderConfig{}, err
	}
	config.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return ProviderConfig{}, err
	}
	return config, nil
}

func parseStoredTime(value string) (time.Time, error) {
	return time.Parse(timeLayout, value)
}

func decodeJSON(payload string, target any) error {
	if strings.TrimSpace(payload) == "" {
		return nil
	}
	return json.Unmarshal([]byte(payload), target)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

const timeLayout = time.RFC3339
