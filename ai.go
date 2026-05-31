package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type AIProvider interface {
	Descriptor() string
	GenerateStructured(ctx context.Context, schemaName, prompt string) (string, error)
	GenerateText(ctx context.Context, prompt string) (string, error)
}

type AIManager struct {
	repo       *Repository
	httpClient *http.Client
}

func NewAIManager(repo *Repository) *AIManager {
	return &AIManager{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (m *AIManager) Provider(ctx context.Context) AIProvider {
	config, err := m.repo.GetProviderConfig(ctx)
	if err == nil && !config.UseMock && config.Enabled && strings.TrimSpace(config.APIKey) != "" {
		return &OpenAICompatibleProvider{config: config, httpClient: m.httpClient}
	}

	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey != "" {
		return &OpenAICompatibleProvider{
			config: ProviderConfig{
				Name:         "Environment Provider",
				ProviderType: "openai-compatible",
				BaseURL:      defaultBaseURL(os.Getenv("OPENAI_BASE_URL")),
				APIKey:       apiKey,
				Model:        defaultModel(os.Getenv("OPENAI_MODEL")),
				APIFormat:    defaultAPIFormat(os.Getenv("OPENAI_API_FORMAT")),
				Enabled:      true,
			},
			httpClient: m.httpClient,
		}
	}
	return &MockProvider{}
}

func (m *AIManager) TestProviderConfig(ctx context.Context, config ProviderConfig) (ProviderConnectionResult, error) {
	result := ProviderConnectionResult{
		Descriptor:      strings.TrimSpace(config.Name),
		BaseURL:         strings.TrimSpace(config.BaseURL),
		ResolvedBaseURL: normalizeProviderBaseURL(config.BaseURL),
		APIFormat:       normalizeAPIFormat(config.APIFormat),
		Model:           defaultModel(config.Model),
	}
	if result.Descriptor == "" {
		result.Descriptor = "openai-compatible"
	}
	if result.APIFormat == "" {
		result.APIFormat = AIAPIFormatResponses
	}
	if strings.TrimSpace(config.APIKey) == "" {
		result.Message = "API Key 不能为空。"
		result.Diagnostic = "请先填写有效密钥，再测试连接。"
		return result, nil
	}
	config.BaseURL = normalizeProviderBaseURL(config.BaseURL)
	config.Model = result.Model
	config.APIFormat = result.APIFormat

	startedAt := time.Now()
	modelsCount, resolvedBaseURL, err := probeModelsEndpoint(ctx, m.httpClient, config)
	result.LatencyMs = time.Since(startedAt).Milliseconds()
	if err != nil {
		result.Message = err.Error()
		result.Diagnostic = "模型列表探测失败，请检查 Base URL、网络和密钥。"
		return result, nil
	}
	result.ResolvedBaseURL = resolvedBaseURL
	result.ModelsCount = modelsCount
	result.OK = true
	result.Message = fmt.Sprintf("连接成功，已探测到 %d 个模型。", modelsCount)
	if resolvedBaseURL != normalizeProviderBaseURL(config.BaseURL) {
		result.Diagnostic = fmt.Sprintf("已自动使用 %s 作为 OpenAI-compatible API 根地址。", resolvedBaseURL)
	} else {
		result.Diagnostic = fmt.Sprintf("当前将按 OpenAI 标准格式直接请求 %s。", resolvedBaseURL)
	}

	provider := &OpenAICompatibleProvider{
		config: ProviderConfig{
			Name:         config.Name,
			ProviderType: config.ProviderType,
			BaseURL:      resolvedBaseURL,
			APIKey:       config.APIKey,
			Model:        config.Model,
			APIFormat:    config.APIFormat,
			Enabled:      config.Enabled,
			UseMock:      config.UseMock,
		},
		httpClient: m.httpClient,
	}
	if _, err := provider.generateContent(ctx, "You are a connectivity probe. Reply with plain text OK only.", "OK"); err != nil {
		result.ContentReady = false
		result.ContentMessage = err.Error()
	} else {
		result.ContentReady = true
		result.ContentMessage = "最小生成探针通过，当前配置可以正常拿到正文。"
	}
	return result, nil
}

type OpenAICompatibleProvider struct {
	config     ProviderConfig
	httpClient *http.Client
}

func (p *OpenAICompatibleProvider) Descriptor() string {
	if p.config.Name != "" {
		return p.config.Name
	}
	return "openai-compatible"
}

func (p *OpenAICompatibleProvider) GenerateStructured(ctx context.Context, schemaName, prompt string) (string, error) {
	systemPrompt := strings.Join([]string{
		"你是一名资深产品经理与需求分析助手。",
		"你必须只输出合法 JSON，不要输出任何额外解释、前后缀或 Markdown 代码块。",
		"输出结果必须严格匹配用户给出的目标结构，并尽量填写完整。",
		"字段值优先使用中文。",
		fmt.Sprintf("当前任务 schema_name=%s。", schemaName),
	}, "\n")
	content, err := p.generateContent(ctx, systemPrompt, prompt)
	if err != nil {
		return "", err
	}
	return extractJSONBlock(content)
}

func (p *OpenAICompatibleProvider) GenerateText(ctx context.Context, prompt string) (string, error) {
	systemPrompt := "你是一名资深产品经理助手，请直接给出清晰、专业、可执行的中文输出。"
	return p.generateContent(ctx, systemPrompt, prompt)
}

func (p *OpenAICompatibleProvider) generateContent(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return p.generateContentWithFormat(ctx, systemPrompt, userPrompt, normalizeAPIFormat(p.config.APIFormat), true)
}

func (p *OpenAICompatibleProvider) generateContentWithFormat(ctx context.Context, systemPrompt, userPrompt string, format AIAPIFormat, allowFallback bool) (string, error) {
	format = normalizeAPIFormat(format)
	if format == "" {
		format = AIAPIFormatResponses
	}

	var (
		endpoint string
		payload  any
	)
	switch format {
	case AIAPIFormatChatCompletions:
		endpoint = "/chat/completions"
		payload = buildChatCompletionsRequestBody(p.config.Model, systemPrompt, userPrompt)
	case AIAPIFormatResponses:
		endpoint = "/responses"
		payload = buildResponsesRequestBody(p.config.Model, systemPrompt, userPrompt)
	default:
		return "", fmt.Errorf("unsupported api format %s", p.config.APIFormat)
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	raw, _, err := executeProviderJSONRequest(ctx, p.httpClient, p.config, http.MethodPost, endpoint, bodyBytes)
	if err != nil {
		return "", err
	}

	switch format {
	case AIAPIFormatChatCompletions:
		text, err := extractChatCompletionsText(raw)
		if err != nil {
			if allowFallback {
				if fallbackText, fallbackErr := p.generateContentWithFormat(ctx, systemPrompt, userPrompt, AIAPIFormatResponses, false); fallbackErr == nil {
					return fallbackText, nil
				} else {
					return "", fmt.Errorf("chat completions returned no readable text: %v; fallback responses also failed: %v", err, fallbackErr)
				}
			}
			return "", fmt.Errorf("chat completions returned no readable text: %w", err)
		}
		return text, nil
	case AIAPIFormatResponses:
		text, err := extractResponsesText(raw)
		if err != nil {
			if allowFallback {
				if fallbackText, fallbackErr := p.generateContentWithFormat(ctx, systemPrompt, userPrompt, AIAPIFormatChatCompletions, false); fallbackErr == nil {
					return fallbackText, nil
				} else {
					return "", fmt.Errorf("responses api returned no readable text: %v; fallback chat completions also failed: %v", err, fallbackErr)
				}
			}
			return "", fmt.Errorf("responses api returned no readable text: %w", err)
		}
		return text, nil
	default:
		return "", fmt.Errorf("unsupported api format %s", p.config.APIFormat)
	}
}

type MockProvider struct{}

func (p *MockProvider) Descriptor() string {
	return "mock-provider"
}

func (p *MockProvider) GenerateStructured(_ context.Context, schemaName, prompt string) (string, error) {
	contextPayload := map[string]any{}
	contextJSON, err := extractJSONBlock(prompt)
	if err != nil {
		contextJSON = "{}"
	}
	if err := json.Unmarshal([]byte(contextJSON), &contextPayload); err != nil {
		contextPayload = map[string]any{}
	}
	var result any
	switch schemaName {
	case "clarifications":
		result = mockClarificationPayload(contextPayload)
	case "consensus_brief":
		result = mockConsensusBrief(contextPayload)
	case "requirement_model":
		result = mockRequirementModel(contextPayload)
	case "prd":
		result = mockPRDDocument(contextPayload)
	case "functional_spec":
		result = mockFunctionalSpec(contextPayload)
	case "technical_spec":
		result = mockTechnicalSpec(contextPayload)
	case "ui_schema":
		result = mockUISchema(contextPayload)
	case "prototype_render_bundle":
		result = mockPrototypeRenderBundle(contextPayload)
	case "prototype_repair_bundle":
		result = mockPrototypeRepairBundle(contextPayload)
	default:
		return "", fmt.Errorf("unknown mock schema %s", schemaName)
	}
	return mustJSON(result), nil
}

func (p *MockProvider) GenerateText(_ context.Context, prompt string) (string, error) {
	return prompt, nil
}

func defaultBaseURL(input string) string {
	return normalizeProviderBaseURL(input)
}

func defaultModel(input string) string {
	if strings.TrimSpace(input) != "" {
		return input
	}
	return "gpt-4.1-mini"
}

func defaultAPIFormat(input string) AIAPIFormat {
	if normalized := normalizeAPIFormat(AIAPIFormat(input)); normalized != "" {
		return normalized
	}
	return AIAPIFormatResponses
}

func buildChatCompletionsRequestBody(model, systemPrompt, userPrompt string) map[string]any {
	return map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.2,
	}
}

func buildResponsesRequestBody(model, systemPrompt, userPrompt string) map[string]any {
	return map[string]any{
		"model":        model,
		"instructions": systemPrompt,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{"type": "input_text", "text": userPrompt},
				},
			},
		},
		"temperature": 0.2,
	}
}

func normalizeProviderBaseURL(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		trimmed = "https://api.openai.com/v1"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return strings.TrimRight(trimmed, "/")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/")
}

type providerEndpointCandidate struct {
	BaseURL string
	URL     string
}

func providerEndpointCandidates(baseURL, endpoint string) []providerEndpointCandidate {
	normalized := normalizeProviderBaseURL(baseURL)
	parsed, err := url.Parse(normalized)
	if err != nil {
		return []providerEndpointCandidate{{BaseURL: normalized, URL: strings.TrimRight(normalized, "/") + endpoint}}
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path != "" {
		return []providerEndpointCandidate{{BaseURL: normalized, URL: strings.TrimRight(normalized, "/") + endpoint}}
	}

	rootBase := strings.TrimRight(normalized, "/")
	withV1 := rootBase + "/v1"
	return []providerEndpointCandidate{
		{BaseURL: rootBase, URL: rootBase + endpoint},
		{BaseURL: withV1, URL: withV1 + endpoint},
	}
}

func executeProviderJSONRequest(ctx context.Context, client *http.Client, config ProviderConfig, method, endpoint string, bodyBytes []byte) (map[string]any, string, error) {
	var lastErr error
	for _, candidate := range providerEndpointCandidates(config.BaseURL, endpoint) {
		var body io.Reader
		if bodyBytes != nil {
			body = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, candidate.URL, body)
		if err != nil {
			lastErr = err
			continue
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+config.APIKey)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed for %s: %w", req.URL.String(), err)
			continue
		}

		responseBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal(responseBody, &raw); err != nil {
			lastErr = fmt.Errorf("provider returned non-JSON response from %s", req.URL.String())
			continue
		}
		if resp.StatusCode >= http.StatusBadRequest {
			if message := extractProviderError(raw); message != "" {
				lastErr = fmt.Errorf("provider error: %s", message)
			} else {
				lastErr = fmt.Errorf("provider returned status %d", resp.StatusCode)
			}
			continue
		}
		return raw, candidate.BaseURL, nil
	}
	if lastErr == nil {
		lastErr = errors.New("provider request failed without a readable response")
	}
	return nil, "", lastErr
}

func probeModelsEndpoint(ctx context.Context, client *http.Client, config ProviderConfig) (int, string, error) {
	payload, resolvedBaseURL, err := executeProviderJSONRequest(ctx, client, config, http.MethodGet, "/models", nil)
	if err != nil {
		return 0, "", err
	}
	models, ok := payload["data"].([]any)
	if !ok {
		return 0, "", errors.New("models endpoint returned an unexpected payload")
	}
	return len(models), resolvedBaseURL, nil
}

func extractProviderError(raw map[string]any) string {
	errorPayload, ok := raw["error"].(map[string]any)
	if !ok {
		return ""
	}
	if message, ok := errorPayload["message"].(string); ok {
		return strings.TrimSpace(message)
	}
	return ""
}

func extractChatCompletionsText(raw map[string]any) (string, error) {
	choices, ok := raw["choices"].([]any)
	if !ok || len(choices) == 0 {
		return "", errors.New("provider returned no choices")
	}
	firstChoice, ok := choices[0].(map[string]any)
	if !ok {
		return "", errors.New("provider returned invalid choices payload")
	}
	message, ok := firstChoice["message"].(map[string]any)
	if !ok {
		return "", errors.New("provider returned no message")
	}
	content, exists := message["content"]
	if !exists {
		return "", errors.New("assistant message has no content field")
	}

	switch content := content.(type) {
	case string:
		if strings.TrimSpace(content) == "" {
			return "", errors.New("assistant message content is empty")
		}
		return content, nil
	case []any:
		var builder strings.Builder
		for _, item := range content {
			switch typed := item.(type) {
			case map[string]any:
				if text, ok := typed["text"].(string); ok {
					if builder.Len() > 0 {
						builder.WriteString("\n")
					}
					builder.WriteString(text)
				}
			}
		}
		if builder.Len() == 0 {
			return "", errors.New("assistant content array did not contain readable text")
		}
		return builder.String(), nil
	default:
		return "", errors.New("provider returned unsupported message content")
	}
}

func extractResponsesText(raw map[string]any) (string, error) {
	output, ok := raw["output"].([]any)
	if ok && len(output) > 0 {
		var builder strings.Builder
		for _, item := range output {
			message, ok := item.(map[string]any)
			if !ok {
				continue
			}
			contentItems, ok := message["content"].([]any)
			if !ok {
				continue
			}
			for _, contentItem := range contentItems {
				contentMap, ok := contentItem.(map[string]any)
				if !ok {
					continue
				}
				contentType, _ := contentMap["type"].(string)
				switch contentType {
				case "output_text", "text":
					if text, ok := contentMap["text"].(string); ok && strings.TrimSpace(text) != "" {
						if builder.Len() > 0 {
							builder.WriteString("\n")
						}
						builder.WriteString(strings.TrimSpace(text))
					}
				}
			}
		}
		if builder.Len() > 0 {
			return builder.String(), nil
		}
	}
	if outputText, ok := raw["output_text"].(string); ok && strings.TrimSpace(outputText) != "" {
		return strings.TrimSpace(outputText), nil
	}
	if !ok || len(output) == 0 {
		return "", errors.New("responses api output array is empty")
	}
	return "", errors.New("responses api output contained no readable text")
}

func mockClarificationPayload(contextPayload map[string]any) map[string]any {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	rawInput := readStringFromMap(contextPayload, "raw_input", "用户希望做一个覆盖产品经理全流程的 AI 工具。")
	goal := fmt.Sprintf("围绕 %s 构建一个从模糊需求到可演示原型的本地优先工作台。", projectName)
	return map[string]any{
		"summary": RequirementSummary{
			ProductName:   projectName,
			TargetUsers:   []string{"产品经理", "业务负责人", "项目发起人"},
			BusinessGoals: []string{goal, "减少需求澄清、写文档和产出原型的时间成本"},
			CoreScenarios: []string{"输入客户模糊需求", "自动产出 PRD 与规格说明", "导出可演示的 React 原型"},
			Constraints:   []string{"本地优先运行", "后续可平滑接入登录与云同步", "第一版只依赖用户输入内容"},
			OpenQuestions: []string{"哪些场景必须在第一版上线", "原型更偏线框还是更偏演示稿"},
		},
		"questions": []ClarificationQuestion{
			{ID: "scope-priority", Question: "第一版最先覆盖哪些核心功能模块？", Rationale: "帮助确定 V1 范围，避免过度设计。", Priority: "high"},
			{ID: "approval-flow", Question: "PRD、功能规格、技术说明是否需要审批状态，还是先只做个人工作台？", Rationale: "影响状态机与后续多人协作设计。", Priority: "medium"},
			{ID: "prototype-style", Question: "导出的原型更偏低保真流程验证，还是需要接近正式前端演示效果？", Rationale: "影响 UISchema 和导出脚手架粒度。", Priority: "high"},
			{ID: "domain-focus", Question: "是否需要针对某个行业领域预置模板，比如电商、SaaS 或内部管理系统？", Rationale: "决定文档模板与术语风格。", Priority: "medium"},
			{ID: "success-metric", Question: "你希望用什么指标判断这个工具是否成功，例如产出速度、文档质量还是沟通效率？", Rationale: "帮助后续定义验收标准。", Priority: "medium"},
		},
		"raw_excerpt": rawInput,
	}
}

func mockPRDDocument(contextPayload map[string]any) ArtifactDocument {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	requirement := readStringFromMap(contextPayload, "raw_input", "围绕产品经理工作流构建一体化 AI 工具。")
	answers := readStringSliceFromMap(contextPayload, "clarification_answers")
	return ArtifactDocument{
		ArtifactType: ArtifactTypePRD,
		Title:        artifactDisplayTitle(projectName, ArtifactTypePRD),
		Summary:      "本 PRD 聚焦用 AI 串联需求澄清、PRD 生成、规格拆解与原型导出，帮助产品经理更快完成从想法到方案的收敛。",
		Sections: []ArtifactSection{
			{ID: "background", Title: "产品背景", Body: requirement, HTML: "<p>" + requirement + "</p>"},
			{ID: "target-users", Title: "目标用户", Body: strings.Join([]string{"1. 产品经理", "2. 业务负责人", "3. 需要快速演示方案的团队成员"}, "\n"), HTML: "<p>产品经理、业务负责人、需要快速演示方案的团队成员。</p>"},
			{ID: "core-flow", Title: "核心流程", Body: strings.Join([]string{"1. 输入模糊需求", "2. 自动提出澄清问题", "3. 生成 PRD", "4. 生成功能规格与研发说明", "5. 生成 UISchema 并导出 React 原型"}, "\n"), HTML: "<p>输入模糊需求，自动提出澄清问题，生成完整文档链路，并落成原型。</p>"},
			{ID: "success-metrics", Title: "成功指标", Body: strings.Join([]string{"单个项目从需求输入到原型导出的时间低于 30 分钟", "文档可编辑并支持版本追溯", "上游文档更新后下游状态可显式提示"}, "\n"), HTML: "<p>以产出速度、版本可追溯性和链路稳定性作为核心衡量标准。</p>"},
			{ID: "risks", Title: "风险与待确认", Body: strings.Join(uniqueStrings(append([]string{"AI 输出结构不稳定", "用户希望导出结果与正式开发代码完全一致"}, answers...)), "\n"), HTML: "<p>主要风险是 AI 输出稳定性以及用户对原型可复用性的预期管理。</p>"},
		},
		Highlights:    []string{"本地优先", "结构化生成", "版本可追踪", "可导出 React 脚手架"},
		OpenQuestions: uniqueStrings(append([]string{"是否需要内置行业模板", "是否需要后续接入云端登录与同步"}, answers...)),
		Metadata: map[string]any{
			"persona_count": 3,
		},
	}
}

func mockConsensusBrief(contextPayload map[string]any) ConsensusBriefData {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	answers := readStringSliceFromMap(contextPayload, "clarification_answers")
	return ConsensusBriefData{
		ProblemStatement: fmt.Sprintf("%s 需要一条从模糊需求到文档和原型的稳定工作流，当前最大的痛点是信息收敛慢、文档反复改和原型产出不稳定。", projectName),
		TargetUsers:      []string{"产品经理", "业务负责人", "项目发起人"},
		BusinessGoals:    []string{"把模糊需求收敛成结构化资产", "缩短 PRD、规格和原型的产出周期"},
		SuccessMetrics:   []string{"单个需求从输入到 PRD 初稿小于 20 分钟", "关键文档和原型都可追溯到上游输入"},
		CoreScenarios:    []string{"输入模糊需求后逐题澄清", "确认共识稿后生成 PRD 和详细规格", "基于页面模型生成可演示原型"},
		InScopeItems:     []string{"需求入口", "澄清流程", "需求共识稿", "PRD/功能规格/技术说明", "页面模型与原型预览"},
		OutOfScopeItems:  []string{"多人审批流", "企业知识库", "云端协同"},
		Risks:            uniqueStrings(append([]string{"AI 输出结构不稳定", "用户对原型可复用性的预期过高"}, answers...)),
		Dependencies:     []string{"稳定的 OpenAI-compatible Provider", "本地 SQLite 持久化"},
		Assumptions:      []string{"第一版以个人产品经理使用为主", "允许人工修改每一层产物"},
	}
}

func mockRequirementModel(contextPayload map[string]any) RequirementModel {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	return RequirementModel{
		ProblemDefinition: RequirementProblemDefinition{
			Summary: fmt.Sprintf("%s 需要把模糊需求稳定收敛成产品资产。", projectName),
			Problem: "当前需求信息散落在文本、会议纪要和口头描述里，导致文档、规格和原型之间经常脱节。",
			Outcome: "建立可追溯的需求真相层，驱动 PRD、功能规格、研发说明和原型统一生成。",
		},
		Actors: []RequirementActor{
			{Name: "产品经理", Responsibilities: []string{"输入需求", "回答澄清问题", "审阅文档"}, PainPoints: []string{"重复写文档", "上下游信息不一致"}},
			{Name: "业务负责人", Responsibilities: []string{"确认目标和范围"}, PainPoints: []string{"需求边界不清晰"}},
		},
		Goals: []string{"统一需求真相层", "缩短方案产出周期", "降低文档与原型偏差"},
		Flows: []RequirementFlow{
			{Name: "需求收敛", Steps: []string{"输入原始需求", "逐题澄清", "确认共识稿"}},
			{Name: "方案产出", Steps: []string{"生成 PRD", "生成功能规格", "生成技术说明", "生成页面模型与原型"}},
		},
		Scope: RequirementScope{
			In:  []string{"需求入口", "澄清流程", "共识稿", "PRD", "功能规格", "技术说明", "原型"},
			Out: []string{"云协作", "审批工作流", "知识库"},
		},
		Entities: []RequirementEntity{
			{Name: "Project", Description: "项目主记录", Fields: []string{"name", "description", "stage"}},
			{Name: "RequirementModel", Description: "需求真相层主对象", Fields: []string{"problem_definition", "actors", "flows", "scope"}},
			{Name: "ArtifactVersion", Description: "文档版本快照", Fields: []string{"status", "structured_json", "source_version_id"}},
		},
		Rules: []RequirementRule{
			{Name: "traceability", Description: "下游产物必须记录其上游来源版本"},
			{Name: "stale-marking", Description: "上游真相层或文档变化时，下游版本必须显式标记 stale"},
		},
		AcceptanceCriteria: []string{"需求共识稿可稳定生成", "RequirementModel 字段完整并可用于下游生成", "老工作流仍可兼容运行"},
		Constraints:        []string{"本地优先", "OpenAI-compatible API", "支持 mock provider 回退"},
		Traceability: map[string]any{
			"source": "consensus_brief",
		},
	}
}

func mockFunctionalSpec(contextPayload map[string]any) ArtifactDocument {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	seed := readFunctionalSpecSeed(contextPayload["functional_spec_seed"])
	if len(seed.Modules) > 0 || len(seed.Pages) > 0 {
		moduleLines := make([]string, 0, len(seed.Modules))
		for _, module := range seed.Modules {
			moduleLines = append(moduleLines, module.Title+"："+module.Summary)
		}
		pageLines := make([]string, 0, len(seed.Pages))
		pageMetadata := make([]map[string]string, 0, len(seed.Pages))
		for _, page := range seed.Pages {
			pageLines = append(pageLines, page.Title+"（"+page.Type+"）："+page.Goal+"；动作："+strings.Join(page.PrimaryActions, "、"))
			pageMetadata = append(pageMetadata, map[string]string{
				"id":    page.ID,
				"title": page.Title,
				"type":  page.Type,
			})
		}
		return ArtifactDocument{
			ArtifactType: ArtifactTypeFunctionalSpec,
			Title:        artifactDisplayTitle(projectName, ArtifactTypeFunctionalSpec),
			Summary:      "功能规格说明现在优先沿用标准化 PRD 和执行种子展开，重点约束模块、页面、字段、状态、异常与验收。",
			Sections: []ArtifactSection{
				{ID: "modules", Title: "模块拆解", Body: strings.Join(moduleLines, "\n"), HTML: "<p>模块拆解已按执行种子约束生成。</p>"},
				{ID: "pages", Title: "页面与流程", Body: strings.Join(pageLines, "\n"), HTML: "<p>页面与流程已按执行种子约束生成。</p>"},
				{ID: "field-rules", Title: "字段与交互规则", Body: strings.Join(seed.FieldRules, "\n"), HTML: "<p>字段与交互规则已按执行种子约束生成。</p>"},
				{ID: "state-rules", Title: "状态规则", Body: strings.Join(seed.StateRules, "\n"), HTML: "<p>状态规则已按执行种子约束生成。</p>"},
				{ID: "exceptions", Title: "异常与兜底", Body: strings.Join(seed.ExceptionRules, "\n"), HTML: "<p>异常与兜底规则已按执行种子约束生成。</p>"},
				{ID: "acceptance", Title: "验收标准", Body: strings.Join(seed.AcceptanceChecklist, "\n"), HTML: "<p>验收标准已按执行种子约束生成。</p>"},
			},
			Metadata: map[string]any{
				"pages":        pageMetadata,
				"traceability": seed.Traceability,
			},
		}
	}
	return ArtifactDocument{
		ArtifactType: ArtifactTypeFunctionalSpec,
		Title:        artifactDisplayTitle(projectName, ArtifactTypeFunctionalSpec),
		Summary:      "功能规格说明把产品能力拆成页面、模块、字段、交互与验收标准，供设计和开发继续落地。",
		Sections: []ArtifactSection{
			{ID: "modules", Title: "模块拆解", Body: strings.Join([]string{"需求输入与澄清", "文档工作台", "版本与状态管理", "原型预览与导出", "AI 配置与运行日志"}, "\n"), HTML: "<p>模块包括需求输入、文档工作台、版本管理、原型预览与导出、AI 配置。</p>"},
			{ID: "pages", Title: "页面与流程", Body: strings.Join([]string{"1. 项目首页", "2. 需求澄清页", "3. PRD 编辑页", "4. 功能规格页", "5. 技术说明页", "6. 原型预览页", "7. AI 设置页"}, "\n"), HTML: "<p>核心页面覆盖项目首页、需求澄清、各类文档编辑和原型预览。</p>"},
			{ID: "field-rules", Title: "字段与交互规则", Body: strings.Join([]string{"需求输入支持长文本保存", "澄清问题支持逐条回答", "文档章节支持单独编辑与保存", "原型预览支持桌面与移动容器切换"}, "\n"), HTML: "<p>关键字段和交互围绕输入、编辑、版本切换与预览切换展开。</p>"},
			{ID: "state-rules", Title: "状态规则", Body: strings.Join([]string{"版本状态包括 draft/generated/reviewed/approved/stale", "上游文档更新时相关下游版本标记 stale", "导出原型时保留最近一次导出结果"}, "\n"), HTML: "<p>状态机以版本流转和失效提醒为核心。</p>"},
			{ID: "acceptance", Title: "验收标准", Body: strings.Join([]string{"可以从模糊需求生成澄清问题", "可以持续生成三类文档与 UISchema", "可以预览并导出 React 原型脚手架"}, "\n"), HTML: "<p>验收重点是闭环可跑通而不是单点文案质量。</p>"},
		},
		Metadata: map[string]any{
			"pages": []map[string]string{
				{"id": "dashboard", "title": "项目首页", "type": "dashboard"},
				{"id": "requirements", "title": "需求澄清页", "type": "form"},
				{"id": "documents", "title": "文档工作台", "type": "list"},
				{"id": "prototype", "title": "原型预览", "type": "detail"},
			},
		},
	}
}

func mockTechnicalSpec(contextPayload map[string]any) ArtifactDocument {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	seed := readTechnicalSpecSeed(contextPayload["technical_spec_seed"])
	if len(seed.Entities) > 0 || len(seed.Interfaces) > 0 {
		entityLines := make([]string, 0, len(seed.Entities))
		for _, entity := range seed.Entities {
			entityLines = append(entityLines, entity.Name+"："+strings.Join(entity.Fields, "、"))
		}
		interfaceLines := make([]string, 0, len(seed.Interfaces))
		for _, item := range seed.Interfaces {
			interfaceLines = append(interfaceLines, item.Name+"："+item.Purpose+"；触发："+item.Trigger)
		}
		return ArtifactDocument{
			ArtifactType: ArtifactTypeTechnicalSpec,
			Title:        artifactDisplayTitle(projectName, ArtifactTypeTechnicalSpec),
			Summary:      "研发说明现在优先沿用标准化 PRD 和技术种子展开，确保实体、接口、状态流转和测试关注点都可追溯。",
			Sections: []ArtifactSection{
				{ID: "entities", Title: "核心实体", Body: strings.Join(entityLines, "\n"), HTML: "<p>核心实体已按技术种子生成。</p>"},
				{ID: "api", Title: "接口与动作", Body: strings.Join(interfaceLines, "\n"), HTML: "<p>接口与动作已按技术种子生成。</p>"},
				{ID: "state", Title: "状态流转", Body: strings.Join(seed.StateTransitions, "\n"), HTML: "<p>状态流转已按技术种子生成。</p>"},
				{ID: "nfr", Title: "非功能要求", Body: strings.Join(seed.NonFunctional, "\n"), HTML: "<p>非功能要求已按技术种子生成。</p>"},
				{ID: "tests", Title: "测试关注点", Body: strings.Join(seed.TestingFocus, "\n"), HTML: "<p>测试关注点已按技术种子生成。</p>"},
			},
			Metadata: map[string]any{
				"traceability": seed.Traceability,
			},
		}
	}
	return ArtifactDocument{
		ArtifactType: ArtifactTypeTechnicalSpec,
		Title:        artifactDisplayTitle(projectName, ArtifactTypeTechnicalSpec),
		Summary:      "研发说明聚焦领域模型、接口、状态流转和导出策略，方便开发直接实现。",
		Sections: []ArtifactSection{
			{ID: "entities", Title: "核心实体", Body: strings.Join([]string{"Project", "RequirementInput", "Artifact", "ArtifactVersion", "WorkflowRun", "ProviderConfig", "UISchema", "PrototypeBundle"}, "\n"), HTML: "<p>核心实体覆盖项目、需求、文档版本、工作流、AI 配置和原型结构。</p>"},
			{ID: "api", Title: "桌面桥接接口", Body: strings.Join([]string{"CreateProject", "SaveRequirementInput", "GenerateClarifications", "GenerateArtifact", "UpdateArtifactSection", "GenerateUISchema", "ExportPrototype"}, "\n"), HTML: "<p>Wails 桥接接口用于串联前端工作台和 Go 服务。</p>"},
			{ID: "state", Title: "状态流转", Body: strings.Join([]string{"生成新版本时递增 version_number", "人工编辑时保留当前版本并更新状态为 draft", "上游变更时下游标记 stale"}, "\n"), HTML: "<p>状态流转关注版本可追溯和下游失效提醒。</p>"},
			{ID: "nfr", Title: "非功能要求", Body: strings.Join([]string{"本地 SQLite 存储", "OpenAI-compatible 模型接入", "无 API Key 时自动回退 Mock", "导出结果落地到本地目录"}, "\n"), HTML: "<p>非功能目标强调本地优先、模型兼容和可用性。</p>"},
			{ID: "tasks", Title: "任务拆解", Body: strings.Join([]string{"1. 建模与存储", "2. AI 编排", "3. 文档编辑界面", "4. UISchema 渲染", "5. React 脚手架导出"}, "\n"), HTML: "<p>研发实施建议按数据层、AI 层、界面层和导出层递进完成。</p>"},
		},
	}
}

func mockUISchema(contextPayload map[string]any) UISchema {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	uiSchemaSeed := readUISchemaSeed(contextPayload["ui_schema_seed"])
	technicalSeed := readTechnicalSpecSeed(contextPayload["technical_spec_seed"])
	if len(uiSchemaSeed.Pages) > 0 {
		return buildSeededUISchema(projectName, uiSchemaSeed, technicalSeed)
	}
	return UISchema{
		AppMeta: UIAppMeta{
			Name:        projectName + " 原型",
			Description: "围绕需求澄清、文档生成和原型导出的桌面工作台原型。",
		},
		Routes: []UIRoute{
			{Path: "/", PageID: "dashboard", Label: "项目总览"},
			{Path: "/requirements", PageID: "requirements", Label: "需求澄清"},
			{Path: "/documents", PageID: "documents", Label: "文档工作台"},
			{Path: "/prototype", PageID: "prototype", Label: "原型预览"},
		},
		Pages: []UIPage{
			{ID: "dashboard", Title: "项目总览", Type: "dashboard", Description: "展示项目进度与生成链路概览", Layout: "two-column", Components: []string{"hero-metrics", "timeline", "todo-list"}, States: []string{"default", "loading", "error"}, PrimaryCTAs: []string{"开始澄清"}, SecondaryCTAs: []string{"查看 PRD"}},
			{ID: "requirements", Title: "需求澄清", Type: "form", Description: "输入原始需求并回答澄清问题", Layout: "single-column", Components: []string{"raw-input", "clarification-form"}, States: []string{"default", "empty", "submitted"}, PrimaryCTAs: []string{"生成 PRD"}, SecondaryCTAs: []string{"保存输入"}},
			{ID: "documents", Title: "文档工作台", Type: "list", Description: "查看和编辑 PRD、功能规格、研发说明", Layout: "three-panel", Components: []string{"artifact-list", "editor", "preview"}, States: []string{"default", "diff", "stale"}, PrimaryCTAs: []string{"批准当前版本"}, SecondaryCTAs: []string{"重生成下游"}},
			{ID: "prototype", Title: "原型预览", Type: "detail", Description: "预览 UISchema 渲染结果并导出 React 脚手架", Layout: "split", Components: []string{"device-switcher", "preview-canvas", "code-pane"}, States: []string{"desktop", "mobile"}, PrimaryCTAs: []string{"导出原型"}, SecondaryCTAs: []string{"刷新预览"}},
		},
		Components: []UIComponent{
			{ID: "hero-metrics", Type: "metrics", PageID: "dashboard", Label: "项目指标", Description: "显示 PRD、规格和原型进度", Props: map[string]any{"cards": []string{"需求完整度", "文档版本", "导出次数"}}},
			{ID: "timeline", Type: "timeline", PageID: "dashboard", Label: "工作流时间线", Description: "显示各阶段进度", Props: map[string]any{"steps": []string{"需求", "PRD", "规格", "原型"}}},
			{ID: "raw-input", Type: "textarea", PageID: "requirements", Label: "原始需求输入", Description: "支持粘贴客户模糊需求", Props: map[string]any{"rows": 8}},
			{ID: "clarification-form", Type: "form", PageID: "requirements", Label: "澄清问题", Description: "逐条回答 AI 生成的问题", Props: map[string]any{"submitLabel": "保存回答"}},
			{ID: "artifact-list", Type: "sidebar", PageID: "documents", Label: "文档列表", Description: "切换不同文档类型与版本", Props: map[string]any{"items": []string{"PRD", "功能规格", "研发说明"}}},
			{ID: "editor", Type: "editor", PageID: "documents", Label: "编辑器", Description: "编辑当前章节内容", Props: map[string]any{"mode": "rich-text"}},
			{ID: "preview", Type: "markdown-preview", PageID: "documents", Label: "预览面板", Description: "显示当前文档渲染结果", Props: map[string]any{"mode": "markdown"}},
			{ID: "device-switcher", Type: "segmented", PageID: "prototype", Label: "设备切换", Description: "桌面与移动预览切换", Props: map[string]any{"options": []string{"desktop", "mobile"}}},
			{ID: "preview-canvas", Type: "canvas", PageID: "prototype", Label: "原型画布", Description: "渲染 UISchema 的页面结构", Props: map[string]any{"defaultRoute": "/"}},
			{ID: "code-pane", Type: "code", PageID: "prototype", Label: "导出代码", Description: "显示导出的 React 脚手架文件", Props: map[string]any{"defaultFile": "src/App.tsx"}},
		},
		Forms: []UIForm{
			{
				ID:        "clarification-form",
				PageID:    "requirements",
				Title:     "澄清问题表单",
				SubmitCTA: "保存回答",
				States:    []string{"default", "submitting", "success"},
				Fields: []UIField{
					{ID: "project-name", Label: "项目名称", Type: "text", Placeholder: "输入项目名称", Required: true},
					{ID: "raw-requirement", Label: "原始需求", Type: "textarea", Placeholder: "粘贴客户需求", Required: true},
					{ID: "priority-goals", Label: "优先目标", Type: "multiselect", Placeholder: "选择优先级", Required: false, Options: []string{"效率", "质量", "协作"}},
				},
			},
		},
		Tables: []UITable{
			{ID: "artifact-table", PageID: "documents", Title: "版本列表", Columns: []string{"文档类型", "版本", "状态", "更新时间"}},
		},
		Actions: []UIAction{
			{ID: "go-requirements", Label: "开始澄清", Kind: "navigate", SourcePageID: "dashboard", TargetPageID: "requirements", Description: "从总览进入澄清页"},
			{ID: "go-documents", Label: "查看 PRD", Kind: "navigate", SourcePageID: "dashboard", TargetPageID: "documents", Description: "查看文档工作台"},
			{ID: "save-answers", Label: "保存回答", Kind: "submit", SourcePageID: "requirements", Description: "提交澄清回答"},
			{ID: "export-prototype", Label: "导出原型", Kind: "export", SourcePageID: "prototype", Description: "导出 React 脚手架"},
		},
		MockData: map[string]any{
			"dashboard_metrics": []map[string]any{
				{"label": "需求完整度", "value": "82%"},
				{"label": "文档版本", "value": 6},
				{"label": "导出次数", "value": 2},
			},
			"artifact_versions": []map[string]any{
				{"type": "PRD", "version": "v3", "status": "approved"},
				{"type": "FunctionalSpec", "version": "v2", "status": "generated"},
				{"type": "TechnicalSpec", "version": "v1", "status": "draft"},
			},
		},
		StateVariants: []UIStateVariant{
			{PageID: "dashboard", State: "loading", Notes: "数据载入中显示骨架屏"},
			{PageID: "documents", State: "stale", Notes: "提示下游版本已过期并可重生成"},
			{PageID: "prototype", State: "mobile", Notes: "移动视口宽度 390px"},
		},
		NavigationMap: []UINavigation{
			{FromPageID: "dashboard", ToPageID: "requirements", Trigger: "开始澄清"},
			{FromPageID: "requirements", ToPageID: "documents", Trigger: "生成 PRD"},
			{FromPageID: "documents", ToPageID: "prototype", Trigger: "查看原型"},
		},
	}
}

func mockPrototypeRenderBundle(contextPayload map[string]any) PrototypeRenderBundleSpec {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	projectDescription := readStringFromMap(contextPayload, "project_description", "AI 原始页面预览")

	uiSchema, err := parseUISchemaJSON(mustJSON(contextPayload["ui_schema"]))
	if err != nil {
		uiSchema = mockUISchema(contextPayload)
	}

	project := Project{
		Name:        projectName,
		Description: projectDescription,
	}
	return PrototypeRenderBundleSpec{
		RenderMode:    prototypeRenderModeAIOriginal,
		DesignSummary: "Mock provider 已模拟生成 AI 原始页面 bundle，预览页会优先显示这份原始页面。",
		AppTSX:        prototypeAppTSX(),
		StylesCSS:     prototypeStylesCSS(),
		PreviewHTML:   prototypePreviewHTML(project, uiSchema),
	}
}

func mockPrototypeRepairBundle(contextPayload map[string]any) PrototypeRenderBundleSpec {
	projectName := readStringFromMap(contextPayload, "project_name", "新产品")
	projectDescription := readStringFromMap(contextPayload, "project_description", "AI 原型修复")

	uiSchema, err := parseUISchemaJSON(mustJSON(contextPayload["ui_schema"]))
	if err != nil {
		uiSchema = mockUISchema(contextPayload)
	}
	currentSpec, err := parsePrototypeRenderBundleJSON(mustJSON(contextPayload["current_render_bundle"]))
	if err != nil {
		currentSpec = mockPrototypeRenderBundle(contextPayload)
	}

	repairRequest := map[string]any{}
	if raw, ok := contextPayload["repair_request"].(map[string]any); ok {
		repairRequest = raw
	}
	input := PrototypeRepairInput{
		PageID:            readStringAny(repairRequest["page_id"], ""),
		CurrentIssue:      readStringAny(repairRequest["current_issue"], "请修复当前原型中的交互问题。"),
		ExpectedBehavior:  readStringAny(repairRequest["expected_behavior"], ""),
		OptimizationNotes: readStringAny(repairRequest["optimization_notes"], ""),
	}

	project := Project{
		Name:        projectName,
		Description: projectDescription,
	}
	spec := localPrototypeRepairBundle(project, uiSchema, currentSpec, input)
	spec.DesignSummary = "Mock provider 已根据当前项目的问题描述生成修复后的原型页面。"
	return spec
}

func buildSeededUISchema(projectName string, seed UISchemaSeed, technicalSeed TechnicalSpecSeed) UISchema {
	description := "围绕页面蓝图、动作种子和状态约束生成的结构化原型。"
	if len(seed.Traceability) > 0 {
		description = seed.Traceability[0]
	}
	return UISchema{
		AppMeta: UIAppMeta{
			Name:        projectName + " 原型",
			Description: description,
		},
		Routes:        buildSeededRoutes(seed.Pages),
		Pages:         buildSeededPages(seed.Pages),
		Components:    buildSeededComponents(seed.Pages),
		Forms:         buildSeededForms(seed.Pages, technicalSeed),
		Tables:        buildSeededTables(seed.Pages, technicalSeed),
		Actions:       buildSeededActions(seed.Actions),
		MockData:      buildSeededMockData(seed, technicalSeed),
		StateVariants: ensureSeedStateVariants(seed),
		NavigationMap: buildSeededNavigation(seed.Actions),
	}
}

func buildSeededRoutes(pages []UISchemaPageSeed) []UIRoute {
	routes := make([]UIRoute, 0, len(pages))
	for index, page := range pages {
		path := "/" + page.ID
		if index == 0 {
			path = "/"
		}
		routes = append(routes, UIRoute{
			Path:   path,
			PageID: page.ID,
			Label:  page.Title,
		})
	}
	return ensureSlice(routes)
}

func buildSeededPages(pages []UISchemaPageSeed) []UIPage {
	items := make([]UIPage, 0, len(pages))
	for _, page := range pages {
		items = append(items, UIPage{
			ID:            page.ID,
			Title:         page.Title,
			Type:          page.Type,
			Description:   page.Goal,
			Layout:        page.Layout,
			Components:    ensureSlice(page.Components),
			States:        ensureSlice(page.States),
			PrimaryCTAs:   ensureSlice(page.PrimaryActions),
			SecondaryCTAs: buildSecondaryCTAs(page),
		})
	}
	return ensureSlice(items)
}

func buildSeededComponents(pages []UISchemaPageSeed) []UIComponent {
	items := []UIComponent{}
	for _, page := range pages {
		for _, component := range page.Components {
			componentID := page.ID + "-" + slugify(component)
			items = append(items, UIComponent{
				ID:          componentID,
				Type:        component,
				PageID:      page.ID,
				Label:       humanizeSeedToken(component),
				Description: page.Title + "中的" + humanizeSeedToken(component),
				Props: map[string]any{
					"source_page": page.Title,
					"data_focus":  ensureSlice(page.DataFocus),
				},
			})
		}
	}
	return ensureSlice(items)
}

func buildSeededForms(pages []UISchemaPageSeed, technicalSeed TechnicalSpecSeed) []UIForm {
	formPages := []UISchemaPageSeed{}
	for _, page := range pages {
		if page.Type == "form" {
			formPages = append(formPages, page)
		}
	}
	fields := buildSeededFields(technicalSeed)
	forms := make([]UIForm, 0, len(formPages))
	for _, page := range formPages {
		forms = append(forms, UIForm{
			ID:        page.ID + "-form",
			PageID:    page.ID,
			Title:     page.Title + "表单",
			Fields:    ensureSlice(fields),
			SubmitCTA: firstOrDefault(page.PrimaryActions, "提交"),
			States:    ensureSlice(page.States),
		})
	}
	return ensureSlice(forms)
}

func buildSeededTables(pages []UISchemaPageSeed, technicalSeed TechnicalSpecSeed) []UITable {
	columns := buildSeededTableColumns(technicalSeed)
	items := []UITable{}
	for _, page := range pages {
		if page.Type != "list" {
			continue
		}
		items = append(items, UITable{
			ID:      page.ID + "-table",
			PageID:  page.ID,
			Title:   page.Title + "数据表",
			Columns: ensureSlice(columns),
		})
	}
	return ensureSlice(items)
}

func buildSeededFields(technicalSeed TechnicalSpecSeed) []UIField {
	if len(technicalSeed.Entities) > 0 {
		entity := technicalSeed.Entities[0]
		fields := make([]UIField, 0, len(entity.Fields))
		for index, field := range limitStrings(entity.Fields, 4) {
			fieldType := "text"
			if index == len(limitStrings(entity.Fields, 4))-1 {
				fieldType = "textarea"
			}
			fields = append(fields, UIField{
				ID:          slugify(field),
				Label:       field,
				Type:        fieldType,
				Placeholder: "请输入" + field,
				Required:    index < 2,
			})
		}
		if len(fields) > 0 {
			return fields
		}
	}
	return []UIField{
		{ID: "name", Label: "名称", Type: "text", Placeholder: "请输入名称", Required: true},
		{ID: "owner", Label: "负责人", Type: "text", Placeholder: "请输入负责人", Required: true},
		{ID: "notes", Label: "补充说明", Type: "textarea", Placeholder: "请输入补充说明", Required: false},
	}
}

func buildSeededTableColumns(technicalSeed TechnicalSpecSeed) []string {
	if len(technicalSeed.Entities) > 0 && len(technicalSeed.Entities[0].Fields) > 0 {
		return limitStrings(technicalSeed.Entities[0].Fields, 5)
	}
	return []string{"名称", "状态", "负责人", "更新时间"}
}

func buildSeededActions(actions []UISchemaActionSeed) []UIAction {
	items := make([]UIAction, 0, len(actions))
	for _, action := range actions {
		items = append(items, UIAction{
			ID:           action.ID,
			Label:        action.Label,
			Kind:         action.Kind,
			SourcePageID: action.SourcePageID,
			TargetPageID: action.TargetPageID,
			Description:  action.Description,
		})
	}
	return ensureSlice(items)
}

func buildSeededMockData(seed UISchemaSeed, technicalSeed TechnicalSpecSeed) map[string]any {
	entityCount := len(technicalSeed.Entities)
	if entityCount == 0 {
		entityCount = len(seed.Pages)
	}
	tableRows := map[string]any{}
	detailRecords := map[string]any{}
	formDefaults := map[string]any{}
	fields := seededEntityFields(technicalSeed)
	for _, page := range seed.Pages {
		switch page.Type {
		case "list":
			tableRows[page.ID+"-table"] = buildSeededRowsForPage(page, fields)
		case "detail":
			detailRecords[page.ID] = buildSeededRecordForPage(page, fields)
		case "form":
			formDefaults[page.ID+"-form"] = buildSeededRecordForPage(page, fields)
		}
	}
	return map[string]any{
		"dashboard_metrics": []map[string]any{
			{"label": "页面数", "value": len(seed.Pages)},
			{"label": "动作数", "value": len(seed.Actions)},
			{"label": "数据焦点", "value": entityCount},
		},
		"table_rows":      tableRows,
		"detail_records":  detailRecords,
		"form_defaults":   formDefaults,
		"mock_data_hints": ensureSlice(seed.MockDataHints),
	}
}

func ensureSeedStateVariants(seed UISchemaSeed) []UIStateVariant {
	if len(seed.StateVariants) > 0 {
		return ensureSlice(seed.StateVariants)
	}
	return []UIStateVariant{}
}

func buildSeededNavigation(actions []UISchemaActionSeed) []UINavigation {
	items := []UINavigation{}
	for _, action := range actions {
		if action.Kind != "navigate" && action.Kind != "submit" {
			continue
		}
		if strings.TrimSpace(action.TargetPageID) == "" {
			continue
		}
		items = append(items, UINavigation{
			FromPageID: action.SourcePageID,
			ToPageID:   action.TargetPageID,
			Trigger:    action.Label,
		})
	}
	return ensureSlice(items)
}

func buildSecondaryCTAs(page UISchemaPageSeed) []string {
	switch page.Type {
	case "dashboard":
		return []string{"查看详情", "浏览进度"}
	case "list":
		return []string{"批量操作", "导出列表"}
	case "form":
		return []string{"保存草稿", "返回上一步"}
	default:
		return []string{"刷新状态", "返回列表"}
	}
}

func humanizeSeedToken(input string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(input), "-", " ")
	normalized = strings.ReplaceAll(normalized, "_", " ")
	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return input
	}
	for index := range parts {
		parts[index] = strings.Title(parts[index])
	}
	return strings.Join(parts, " ")
}

func seededEntityFields(technicalSeed TechnicalSpecSeed) []string {
	if len(technicalSeed.Entities) > 0 && len(technicalSeed.Entities[0].Fields) > 0 {
		return limitStrings(technicalSeed.Entities[0].Fields, 5)
	}
	return []string{"name", "status", "owner", "updated_at"}
}

func buildSeededRowsForPage(page UISchemaPageSeed, fields []string) []map[string]any {
	rows := make([]map[string]any, 0, 3)
	for index := 0; index < 3; index++ {
		record := buildSeededRecordForPage(page, fields)
		for key := range record {
			record[key] = sampleValueForField(key, index, page.Title)
		}
		rows = append(rows, record)
	}
	return rows
}

func buildSeededRecordForPage(page UISchemaPageSeed, fields []string) map[string]any {
	record := map[string]any{}
	for _, field := range fields {
		record[field] = sampleValueForField(field, 0, page.Title)
	}
	if len(record) == 0 {
		record["title"] = page.Title
		record["status"] = "active"
	}
	return record
}

func sampleValueForField(field string, index int, pageTitle string) any {
	normalized := strings.ToLower(strings.TrimSpace(field))
	switch {
	case strings.Contains(normalized, "id"):
		return fmt.Sprintf("%s-%03d", slugify(pageTitle), index+1)
	case strings.Contains(normalized, "status"), strings.Contains(field, "状态"):
		options := []string{"active", "pending", "reviewing"}
		return options[index%len(options)]
	case strings.Contains(normalized, "owner"), strings.Contains(field, "负责人"):
		owners := []string{"产品经理", "业务负责人", "运营同学"}
		return owners[index%len(owners)]
	case strings.Contains(normalized, "time"), strings.Contains(normalized, "date"), strings.Contains(normalized, "updated"), strings.Contains(field, "时间"):
		dates := []string{"今天 10:30", "昨天 18:20", "04-10 09:45"}
		return dates[index%len(dates)]
	case strings.Contains(normalized, "name"), strings.Contains(normalized, "title"), strings.Contains(field, "名称"):
		return fmt.Sprintf("%s示例%d", pageTitle, index+1)
	default:
		return fmt.Sprintf("%s值%d", humanizeSeedToken(field), index+1)
	}
}

func readStringFromMap(payload map[string]any, key, fallback string) string {
	if value, ok := payload[key]; ok {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return fallback
}

func readStringSliceFromMap(payload map[string]any, key string) []string {
	var result []string
	raw, ok := payload[key]
	if !ok {
		return result
	}
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
	case []string:
		result = append(result, typed...)
	}
	return uniqueStrings(result)
}
