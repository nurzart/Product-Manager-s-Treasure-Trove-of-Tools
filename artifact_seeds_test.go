package main

import "testing"

func TestUnit_BuildFunctionalSpecSeed_FromStandardizedPRD(t *testing.T) {
	baseline := map[string]any{
		"sections": []map[string]any{
			{"key": "goals", "content": "提升客户管理效率。\n缩短录入周期。"},
			{"key": "features", "content": "客户总览\n客户列表\n新建客户\n客户详情"},
			{"key": "acceptance", "content": "必须覆盖列表筛选。\n必须覆盖表单校验。"},
			{"key": "scope", "content": "第一版只做客户管理与详情查看。"},
		},
	}
	model := &RequirementModel{
		Entities: []RequirementEntity{
			{Name: "Customer", Fields: []string{"id", "name", "owner", "status"}},
		},
		AcceptanceCriteria: []string{"能够查看客户详情。"},
	}

	seed := buildFunctionalSpecSeed(Project{Name: "客户管理"}, baseline, nil, nil, model)
	if len(seed.Modules) < 3 {
		t.Fatalf("expected modules to be derived from features, got %+v", seed.Modules)
	}
	if len(seed.Pages) < 3 {
		t.Fatalf("expected pages to be derived from modules, got %+v", seed.Pages)
	}
	if seed.Pages[1].Type != "list" {
		t.Fatalf("expected second page to be classified as list, got %+v", seed.Pages[1])
	}
	if len(seed.FieldRules) == 0 || len(seed.AcceptanceChecklist) == 0 {
		t.Fatalf("expected field rules and acceptance checklist, got %+v", seed)
	}
}

func TestUnit_BuildTechnicalSpecSeed_FromFunctionalSeed(t *testing.T) {
	functionalSeed := FunctionalSpecSeed{
		Modules: []FunctionalModuleSeed{
			{ID: "customer-list", Title: "客户列表", Summary: "查看和筛选客户"},
		},
		Pages: []FunctionalPageSeed{
			{ID: "customer-list", Title: "客户列表", Type: "list", Goal: "查看和筛选客户", PrimaryActions: []string{"筛选", "查看详情"}, States: []string{"default", "loading", "empty", "error"}},
		},
		AcceptanceChecklist: []string{"列表支持筛选"},
	}

	seed := buildTechnicalSpecSeed(Project{Name: "客户管理"}, map[string]any{}, nil, nil, functionalSeed)
	if len(seed.Interfaces) == 0 {
		t.Fatalf("expected interfaces to be derived from functional seed, got %+v", seed)
	}
	if len(seed.StateTransitions) == 0 || len(seed.TestingFocus) == 0 {
		t.Fatalf("expected state transitions and testing focus, got %+v", seed)
	}
}

func TestUnit_BuildUISchemaSeed_FromExecutionSeeds(t *testing.T) {
	functionalSeed := FunctionalSpecSeed{
		Pages: []FunctionalPageSeed{
			{ID: "customer-overview", Title: "客户总览页", Type: "dashboard", Goal: "查看客户与进度指标", PrimaryActions: []string{"进入客户列表"}, States: []string{"default", "loading"}},
			{ID: "customer-list", Title: "客户列表页", Type: "list", Goal: "查看和筛选客户", PrimaryActions: []string{"筛选", "查看详情"}, States: []string{"default", "loading", "empty"}},
			{ID: "customer-form", Title: "客户表单页", Type: "form", Goal: "创建客户", PrimaryActions: []string{"提交"}, States: []string{"default", "submitting", "success"}},
		},
		Traceability: []string{"页面蓝图来源于功能规格。"},
	}
	technicalSeed := TechnicalSpecSeed{
		Entities: []RequirementEntity{
			{Name: "Customer", Fields: []string{"id", "name", "owner", "status"}},
		},
		Interfaces: []TechnicalInterfaceSeed{
			{Name: "CustomerListQuery", SourcePageID: "customer-list", Purpose: "加载客户列表", Trigger: "进入列表页"},
		},
		Traceability: []string{"接口动作来源于研发说明。"},
	}

	seed := buildUISchemaSeed(functionalSeed, technicalSeed)
	if len(seed.Pages) != 3 {
		t.Fatalf("expected 3 ui schema seed pages, got %+v", seed.Pages)
	}
	if seed.Pages[1].Layout != "content-plus-side" {
		t.Fatalf("expected list page layout to be content-plus-side, got %+v", seed.Pages[1])
	}
	if len(seed.Actions) == 0 {
		t.Fatalf("expected ui schema seed actions to be derived, got %+v", seed.Actions)
	}
	if len(seed.StateVariants) == 0 || len(seed.MockDataHints) == 0 {
		t.Fatalf("expected state variants and mock data hints, got %+v", seed)
	}
}
