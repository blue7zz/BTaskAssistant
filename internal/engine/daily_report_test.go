package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func dailyReportGenerationTestInput() DailyReportGenerationInput {
	return DailyReportGenerationInput{
		RequestID:          "daily-report-engine-test",
		ReportDate:         "2026-07-30",
		Organization:       "万象",
		Level:              "L1",
		Role:               "FE",
		Engine:             "pi",
		CustomInstructions: "保持简洁",
		GitAuthor:          "Daily Tester",
		IncludeUncommitted: true,
		ManualDescription:  "参加项目评审并确认验收标准，明天完成三个回归问题",
		WorkflowTasks: []DailyReportGenerationWorkflowTask{{
			TaskID:            "task-001",
			Title:             "实现日报填报",
			ProjectName:       "BTaskAssistant",
			Status:            "review",
			DevelopmentState:  "completed",
			Summary:           "增加 AI 日报生成入口",
			DevelopmentResult: "已完成后端生成契约",
			ReviewNote:        "等待界面联调",
			UpdatedAt:         "2026-07-30T10:30:00Z",
		}},
	}
}

func TestNormalizeDailyReportGenerationInputAllowsMissingProjectNumber(t *testing.T) {
	input := dailyReportGenerationTestInput()
	input.Projects = []DailyReportGenerationProject{{
		ProjectName: "y15_app",
		Path:        "/workspace/y15_app",
	}}

	normalized, err := normalizeDailyReportGenerationInput(input)
	if err != nil {
		t.Fatalf("normalize generation input: %v", err)
	}
	if normalized.Projects[0].ProjectNo != "" {
		t.Fatalf("unexpected inferred project number %q", normalized.Projects[0].ProjectNo)
	}
}

func TestNormalizeDailyReportGenerationInputRejectsInvalidBounds(t *testing.T) {
	input := dailyReportGenerationTestInput()
	input.ReportDate = "2026-02-30"
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("expected invalid date error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.Projects = make([]DailyReportGenerationProject, 9)
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "最多选择 8 个") {
		t.Fatalf("expected project limit error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.CustomInstructions = strings.Repeat("x", maxDailyReportCustomInstructions+1)
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "32 KB") {
		t.Fatalf("expected instruction limit error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.ManualDescription = strings.Repeat("x", maxDailyReportManualDescription+1)
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "人工描述") {
		t.Fatalf("expected manual description limit error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.WorkflowTasks = make([]DailyReportGenerationWorkflowTask, 31)
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "最多选择 30 个") {
		t.Fatalf("expected workflow task limit error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.WorkflowTasks = append(input.WorkflowTasks, input.WorkflowTasks[0])
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "ID 重复") {
		t.Fatalf("expected duplicate workflow task error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.WorkflowTasks[0].Status = "cancelled"
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "状态不受支持") {
		t.Fatalf("expected workflow status error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.WorkflowTasks[0].DevelopmentState = "failed"
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "开发状态不受支持") {
		t.Fatalf("expected workflow development state error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.WorkflowTasks[0].UpdatedAt = "today"
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "更新时间格式不正确") {
		t.Fatalf("expected workflow timestamp error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.WorkflowTasks[0].Summary = strings.Repeat("x", maxDailyReportWorkflowTaskText+1)
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "任务摘要") {
		t.Fatalf("expected workflow task text limit error, got %v", err)
	}

	input = dailyReportGenerationTestInput()
	input.ManualDescription = ""
	input.WorkflowTasks = nil
	if _, err := normalizeDailyReportGenerationInput(input); err == nil ||
		!strings.Contains(err.Error(), "Git 项目、工作流任务或填写人工描述") {
		t.Fatalf("expected empty sources error, got %v", err)
	}
}

func TestNormalizeDailyReportGenerationInputAllowsAnySingleSource(t *testing.T) {
	cases := map[string]DailyReportGenerationInput{}

	projectOnly := dailyReportGenerationTestInput()
	projectOnly.ManualDescription = ""
	projectOnly.WorkflowTasks = nil
	projectOnly.Projects = []DailyReportGenerationProject{{
		ProjectName: "BTaskAssistant",
		Path:        "/workspace/BTaskAssistant",
	}}
	cases["Git project"] = projectOnly

	manualOnly := dailyReportGenerationTestInput()
	manualOnly.WorkflowTasks = nil
	cases["manual description"] = manualOnly

	workflowTaskOnly := dailyReportGenerationTestInput()
	workflowTaskOnly.ManualDescription = ""
	cases["workflow task"] = workflowTaskOnly

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeDailyReportGenerationInput(input); err != nil {
				t.Fatalf("normalize %s input: %v", name, err)
			}
		})
	}
}

func TestNormalizeDailyReportGenerationInputAcceptsCurrentWorkflowStatuses(t *testing.T) {
	for _, status := range []string{
		"inbox",
		"requirements",
		"approved",
		"development",
		"review",
		"done",
	} {
		t.Run(status, func(t *testing.T) {
			input := dailyReportGenerationTestInput()
			input.WorkflowTasks[0].Status = status
			if _, err := normalizeDailyReportGenerationInput(input); err != nil {
				t.Fatalf("normalize status %q: %v", status, err)
			}
		})
	}
}

func TestNormalizeDailyReportGenerationInputAcceptsDevelopmentStates(t *testing.T) {
	for _, state := range []string{"idle", "delegated", "completed"} {
		t.Run(state, func(t *testing.T) {
			input := dailyReportGenerationTestInput()
			input.WorkflowTasks[0].DevelopmentState = state
			if _, err := normalizeDailyReportGenerationInput(input); err != nil {
				t.Fatalf("normalize development state %q: %v", state, err)
			}
		})
	}
}

func TestNormalizeDailyReportGenerationInputSanitizesWorkflowText(t *testing.T) {
	input := dailyReportGenerationTestInput()
	input.ManualDescription = "查看 /Users/blue/private 并联系 owner@example.com"
	input.WorkflowTasks[0].Title = "处理 /workspace/private 中的问题"
	input.WorkflowTasks[0].ProjectName = "https://secret.example/project"
	input.WorkflowTasks[0].Summary = "远端 git@example.com:private/repo.git"
	input.WorkflowTasks[0].DevelopmentResult = "产物 C:\\private\\output"
	input.WorkflowTasks[0].ReviewNote = "记录 reviewer@example.com"

	normalized, err := normalizeDailyReportGenerationInput(input)
	if err != nil {
		t.Fatalf("normalize sensitive workflow text: %v", err)
	}
	task := normalized.WorkflowTasks[0]
	combined := strings.Join([]string{
		normalized.ManualDescription,
		task.Title,
		task.ProjectName,
		task.Summary,
		task.DevelopmentResult,
		task.ReviewNote,
	}, "\n")
	for _, forbidden := range []string{
		"/Users/blue/private",
		"/workspace/private",
		"https://secret.example/project",
		"git@example.com:private/repo.git",
		`C:\private\output`,
		"owner@example.com",
		"reviewer@example.com",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("normalized workflow text leaked %q:\n%s", forbidden, combined)
		}
	}
	if task.TaskID != input.WorkflowTasks[0].TaskID {
		t.Fatalf("task ID should remain available for deduplication: %q", task.TaskID)
	}
}

func TestBuildDailyReportGenerationPromptExcludesLocalIdentityAndPaths(t *testing.T) {
	input := dailyReportGenerationTestInput()
	input.GitAuthor = "daily.tester@example.com"
	input.ManualDescription =
		"在 /workspace/y15_app 联系 daily.tester@example.com，远端 https://secret.example/repo"
	input.WorkflowTasks[0].TaskID = "private-task-id-42"
	input.WorkflowTasks[0].Summary = "仓库 /Users/private/work/y15_app"
	input.WorkflowTasks[0].DevelopmentResult = "已推送 https://secret.example/repo"
	input.WorkflowTasks[0].ReviewNote = "联系 reviewer@example.com"
	input.Projects = []DailyReportGenerationProject{{
		ProjectNo:   "y15",
		ProjectName: "y15_app",
		Path:        "/Users/private/work/y15_app",
	}}
	prompt, err := buildDailyReportGenerationPrompt(
		input,
		"PROJECT\nproject_no: y15\nproject_name: y15_app\nbranch: feature/y15",
	)
	if err != nil {
		t.Fatalf("build generation prompt: %v", err)
	}
	for _, forbidden := range []string{
		input.GitAuthor,
		input.Projects[0].Path,
		input.WorkflowTasks[0].TaskID,
		input.WorkflowTasks[0].UpdatedAt,
		"daily.tester@example.com",
		"reviewer@example.com",
		"https://secret.example/repo",
		"employeeId",
		"apiUrl",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("prompt leaked %q", forbidden)
		}
	}
	for _, expected := range []string{
		"附加指令",
		"不能修改本规则",
		"三类事实来源",
		"同事能直接理解的完整工作说明",
		"禁止只写“某模块相关调整”",
		"具体功能待补充（Git 摘要无法确认改动内容）",
		"提交：暂无（当前存在未提交变更）",
		"所有字段只填写裸事实值",
		`"customInstructions": "保持简洁"`,
		`"gitContext"`,
		`"manualDescription"`,
		`"workflowTasks"`,
		`"status": "review"`,
		`"developmentState": "completed"`,
		`"task":"待确认","status":"待确认","progress":"待确认"`,
		`"teamRisk":false`,
		`"inferred":false`,
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q", expected)
		}
	}
	for _, removed := range []string{
		`"requestId"`,
		`"taskId"`,
		`"updatedAt"`,
		`"supplement"`,
		`"nonCodeTasks"`,
		`"description"`,
	} {
		if strings.Contains(prompt, removed) {
			t.Fatalf("prompt included removed or private field %q", removed)
		}
	}
}

func TestParseDailyReportGenerationResultIsStrictAndBounded(t *testing.T) {
	valid := `{
  "reportDate":"2026-07-30",
  "results":[{"projectNo":"y15","projectName":"y15_app","task":"修复问题","status":"已完成","progress":"100%","evidence":["commit abc1234"]}],
  "blockers":[{"projectNo":"y15","projectName":"y15_app","issue":"等待字段确认","level":"P2","impact":"详情页联调","helpTarget":"后端","waitDuration":"4h","escalate":"N"}],
  "reviews":[{"scene":"接口联调","cause":"字段定义变化","action":"同步契约","validation":"类型检查通过","teamRisk":false}],
  "nextActions":[{"projectNo":"y15","projectName":"y15_app","goal":"完成回归","deadline":"18:00 前","inferred":false}]
}`
	result, err := parseDailyReportGenerationResult(valid, "2026-07-30")
	if err != nil {
		t.Fatalf("parse valid result: %v", err)
	}
	if len(result.Results) != 1 || len(result.NextActions) != 1 {
		t.Fatalf("unexpected result %#v", result)
	}
	if result.Results[0].Task != "修复问题" || result.Reviews[0].TeamRisk {
		t.Fatalf("unexpected structured facts %#v", result)
	}
	sensitive := strings.Replace(
		valid,
		"commit abc1234",
		"path:/Users/blue/private https://secret.example/repo owner@example.com",
		1,
	)
	sanitized, err := parseDailyReportGenerationResult(sensitive, "2026-07-30")
	if err != nil {
		t.Fatalf("parse sensitive result: %v", err)
	}
	for _, forbidden := range []string{
		"/Users/blue/private",
		"https://secret.example/repo",
		"owner@example.com",
	} {
		if strings.Contains(sanitized.Results[0].Evidence[0], forbidden) {
			t.Fatalf(
				"generated result leaked %q: %s",
				forbidden,
				sanitized.Results[0].Evidence[0],
			)
		}
	}

	if _, err := parseDailyReportGenerationResult(valid+" trailing", "2026-07-30"); err == nil {
		t.Fatal("expected trailing content to be rejected")
	}
	unknown := strings.Replace(valid, `"results":`, `"unknown":true,"results":`, 1)
	if _, err := parseDailyReportGenerationResult(unknown, "2026-07-30"); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
	missing := `{"reportDate":"2026-07-30","results":[]}`
	if _, err := parseDailyReportGenerationResult(missing, "2026-07-30"); err == nil ||
		!strings.Contains(err.Error(), "缺少固定字段") {
		t.Fatalf("expected missing field error, got %v", err)
	}
	nestedUnknown := strings.Replace(valid, `"task":"修复问题"`, `"task":"修复问题","description":"旧字段"`, 1)
	if _, err := parseDailyReportGenerationResult(nestedUnknown, "2026-07-30"); err == nil {
		t.Fatal("expected nested unknown field to be rejected")
	}
	missingTeamRisk := strings.Replace(valid, `,"teamRisk":false`, "", 1)
	if _, err := parseDailyReportGenerationResult(missingTeamRisk, "2026-07-30"); err == nil ||
		!strings.Contains(err.Error(), "teamRisk") {
		t.Fatalf("expected missing teamRisk error, got %v", err)
	}
	missingInferred := strings.Replace(valid, `,"inferred":false`, "", 1)
	if _, err := parseDailyReportGenerationResult(missingInferred, "2026-07-30"); err == nil ||
		!strings.Contains(err.Error(), "inferred") {
		t.Fatalf("expected missing inferred error, got %v", err)
	}
	fourActions := strings.Replace(
		valid,
		`"nextActions":[{"projectNo":"y15","projectName":"y15_app","goal":"完成回归","deadline":"18:00 前","inferred":false}]`,
		`"nextActions":[
 {"projectNo":"y15","projectName":"y15_app","goal":"目标1","deadline":"10:00","inferred":false},
 {"projectNo":"y15","projectName":"y15_app","goal":"目标2","deadline":"11:00","inferred":false},
 {"projectNo":"y15","projectName":"y15_app","goal":"目标3","deadline":"12:00","inferred":false},
 {"projectNo":"y15","projectName":"y15_app","goal":"目标4","deadline":"13:00","inferred":false}
]`,
		1,
	)
	if _, err := parseDailyReportGenerationResult(fourActions, "2026-07-30"); err == nil ||
		!strings.Contains(err.Error(), "TOP3") {
		t.Fatalf("expected TOP3 limit error, got %v", err)
	}

	buffer := newCappedBuffer(4)
	_, _ = buffer.Write([]byte("12345"))
	if !buffer.exceeded || buffer.String() != "1234" {
		t.Fatalf("output cap failed: exceeded=%v value=%q", buffer.exceeded, buffer.String())
	}
}

func TestParseDailyReportGenerationResultRejectsInvalidFacts(t *testing.T) {
	base := `{
  "reportDate":"2026-07-30",
  "results":[{"projectNo":"y15","projectName":"y15_app","task":"修复问题","status":"已完成","progress":"100%","evidence":["commit abc1234"]}],
  "blockers":[{"projectNo":"y15","projectName":"y15_app","issue":"等待字段确认","level":"P2","impact":"详情页联调","helpTarget":"后端","waitDuration":"4h","escalate":"N"}],
  "reviews":[{"scene":"接口联调","cause":"字段定义变化","action":"同步契约","validation":"类型检查通过","teamRisk":true}],
  "nextActions":[{"projectNo":"y15","projectName":"y15_app","goal":"完成回归","deadline":"18:00 前","inferred":true}]
}`
	tests := []struct {
		name      string
		oldValue  string
		newValue  string
		errorPart string
	}{
		{name: "result status", oldValue: `"status":"已完成"`, newValue: `"status":"完成"`, errorPart: "状态必须是"},
		{name: "progress range", oldValue: `"progress":"100%"`, newValue: `"progress":"101%"`, errorPart: "进度必须"},
		{name: "blocker level", oldValue: `"level":"P2"`, newValue: `"level":"P3"`, errorPart: "等级必须是"},
		{name: "escalate", oldValue: `"escalate":"N"`, newValue: `"escalate":"false"`, errorPart: "是否升级必须是"},
		{name: "internal Git source evidence", oldValue: `"commit abc1234"`, newValue: `"gitContext.branch"`, errorPart: "内部来源标识"},
		{name: "internal manual source evidence", oldValue: `"commit abc1234"`, newValue: `"manualDescription"`, errorPart: "内部来源标识"},
		{name: "internal workflow source evidence", oldValue: `"commit abc1234"`, newValue: `"workflowTasks.status"`, errorPart: "内部来源标识"},
		{name: "internal summary assignment", oldValue: `"commit abc1234"`, newValue: `"summary: 内部摘要"`, errorPart: "内部字段赋值"},
		{name: "internal summary assignment with Chinese colon", oldValue: `"commit abc1234"`, newValue: `"summary：内部摘要"`, errorPart: "内部字段赋值"},
		{name: "internal project assignment", oldValue: `"commit abc1234"`, newValue: `"projectName=Y16"`, errorPart: "内部字段赋值"},
		{name: "preformatted result task", oldValue: `"task":"修复问题"`, newValue: `"task":"修复问题；状态：已完成"`, errorPart: "展示标签"},
		{name: "preformatted blocker issue", oldValue: `"issue":"等待字段确认"`, newValue: `"issue":"等待字段确认；等级：P2"`, errorPart: "展示标签"},
		{name: "preformatted review cause", oldValue: `"cause":"字段定义变化"`, newValue: `"cause":"根因：字段定义变化"`, errorPart: "展示标签"},
		{name: "preformatted review action", oldValue: `"action":"同步契约"`, newValue: `"action":"处置：同步契约"`, errorPart: "展示标签"},
		{name: "preformatted review validation", oldValue: `"validation":"类型检查通过"`, newValue: `"validation":"验证结果：类型检查通过"`, errorPart: "展示标签"},
		{name: "preformatted next goal", oldValue: `"goal":"完成回归"`, newValue: `"goal":"TOP 1：完成回归"`, errorPart: "展示标签"},
		{name: "preformatted deadline", oldValue: `"deadline":"18:00 前"`, newValue: `"deadline":"（截止：18:00 前）"`, errorPart: "展示标签"},
		{name: "preformatted deadline label", oldValue: `"deadline":"18:00 前"`, newValue: `"deadline":"截止时间：18:00 前"`, errorPart: "展示标签"},
		{name: "preformatted evidence", oldValue: `"commit abc1234"`, newValue: `"证据：commit abc1234"`, errorPart: "展示标签"},
		{name: "preformatted impact", oldValue: `"impact":"详情页联调"`, newValue: `"impact":"影响：详情页联调"`, errorPart: "展示标签"},
		{name: "preformatted help target", oldValue: `"helpTarget":"后端"`, newValue: `"helpTarget":"求助对象：后端"`, errorPart: "展示标签"},
		{name: "preformatted wait duration", oldValue: `"waitDuration":"4h"`, newValue: `"waitDuration":"等待时长：4h"`, errorPart: "展示标签"},
		{name: "result task length", oldValue: `"task":"修复问题"`, newValue: `"task":"` + strings.Repeat("x", 2001) + `"`, errorPart: "任务不能超过 2000 个字节"},
		{name: "blocker issue length", oldValue: `"issue":"等待字段确认"`, newValue: `"issue":"` + strings.Repeat("x", 2001) + `"`, errorPart: "卡点不能超过 2000 个字节"},
		{name: "review cause length", oldValue: `"cause":"字段定义变化"`, newValue: `"cause":"` + strings.Repeat("x", 2001) + `"`, errorPart: "根因不能超过 2000 个字节"},
		{name: "next goal length", oldValue: `"goal":"完成回归"`, newValue: `"goal":"` + strings.Repeat("x", 1001) + `"`, errorPart: "目标不能超过 1000 个字节"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := strings.Replace(base, test.oldValue, test.newValue, 1)
			if _, err := parseDailyReportGenerationResult(value, "2026-07-30"); err == nil ||
				!strings.Contains(err.Error(), test.errorPart) {
				t.Fatalf("expected %q error, got %v", test.errorPart, err)
			}
		})
	}

	for _, evidence := range []string{
		"status=done",
		"status：inbox",
		"summary=完成日报",
		"summary：内部摘要",
		"taskId=task-001",
		"updatedAt=2026-07-30",
		"developmentState=completed",
		"developmentResult=测试通过",
		"reviewNote=已审核",
	} {
		t.Run("internal evidence assignment "+evidence, func(t *testing.T) {
			value := strings.Replace(base, "commit abc1234", evidence, 1)
			if _, err := parseDailyReportGenerationResult(value, "2026-07-30"); err == nil ||
				!strings.Contains(err.Error(), "内部字段赋值") {
				t.Fatalf("expected internal evidence assignment error, got %v", err)
			}
		})
	}

	newlineFields := []struct {
		name     string
		oldValue string
		newValue string
	}{
		{name: "task", oldValue: `"task":"修复问题"`, newValue: `"task":"修复\n问题"`},
		{name: "issue", oldValue: `"issue":"等待字段确认"`, newValue: `"issue":"等待\n字段确认"`},
		{name: "impact", oldValue: `"impact":"详情页联调"`, newValue: `"impact":"详情页\n联调"`},
		{name: "scene", oldValue: `"scene":"接口联调"`, newValue: `"scene":"接口\n联调"`},
		{name: "cause", oldValue: `"cause":"字段定义变化"`, newValue: `"cause":"字段\n变化"`},
		{name: "action", oldValue: `"action":"同步契约"`, newValue: `"action":"同步\n契约"`},
		{name: "validation", oldValue: `"validation":"类型检查通过"`, newValue: `"validation":"类型检查\n通过"`},
		{name: "goal", oldValue: `"goal":"完成回归"`, newValue: `"goal":"完成\n回归"`},
	}
	for _, field := range newlineFields {
		t.Run("newline "+field.name, func(t *testing.T) {
			value := strings.Replace(base, field.oldValue, field.newValue, 1)
			if _, err := parseDailyReportGenerationResult(value, "2026-07-30"); err == nil ||
				!strings.Contains(err.Error(), "必须为单行文本") {
				t.Fatalf("expected single-line %s error, got %v", field.name, err)
			}
		})
	}

	placeholderMixed := strings.Replace(
		base,
		`"evidence":["commit abc1234"]`,
		`"evidence":["待确认","commit abc1234"]`,
		1,
	)
	if _, err := parseDailyReportGenerationResult(placeholderMixed, "2026-07-30"); err == nil ||
		!strings.Contains(err.Error(), "唯一占位项") {
		t.Fatalf("expected placeholder uniqueness error, got %v", err)
	}

	missingEvidence := strings.Replace(base, `"evidence":["commit abc1234"]`, `"evidence":[]`, 1)
	if _, err := parseDailyReportGenerationResult(missingEvidence, "2026-07-30"); err == nil ||
		!strings.Contains(err.Error(), "证据不能为空") {
		t.Fatalf("expected empty evidence error, got %v", err)
	}

	legitimateStatusEvidence := strings.Replace(
		base,
		"commit abc1234",
		"接口返回 status 200，联调验证通过",
		1,
	)
	if _, err := parseDailyReportGenerationResult(
		legitimateStatusEvidence,
		"2026-07-30",
	); err != nil {
		t.Fatalf("generic status evidence should be accepted: %v", err)
	}

	pendingFacts := strings.NewReplacer(
		`"status":"已完成"`, `"status":"待确认"`,
		`"progress":"100%"`, `"progress":"待确认"`,
		`"evidence":["commit abc1234"]`, `"evidence":["待确认"]`,
		`"level":"P2"`, `"level":"待确认"`,
		`"escalate":"N"`, `"escalate":"待确认"`,
	).Replace(base)
	if _, err := parseDailyReportGenerationResult(pendingFacts, "2026-07-30"); err != nil {
		t.Fatalf("pending placeholders should be accepted: %v", err)
	}
}

func TestParseDailyReportGenerationResultRejectsCollectionLimits(t *testing.T) {
	resultItem := `{"projectNo":"y15","projectName":"y15_app","task":"任务","status":"进行中","progress":"50%","evidence":["分支 feature/y15"]}`
	blockerItem := `{"projectNo":"y15","projectName":"y15_app","issue":"卡点","level":"一般","impact":"联调","helpTarget":"后端","waitDuration":"1h","escalate":"N"}`
	reviewItem := `{"scene":"联调","cause":"字段变化","action":"同步契约","validation":"检查通过","teamRisk":false}`
	base := `{"reportDate":"2026-07-30","results":[],"blockers":[],"reviews":[],"nextActions":[]}`
	tests := []struct {
		name      string
		field     string
		items     string
		errorPart string
	}{
		{name: "results", field: `"results":[]`, items: `"results":[` + repeatedJSONItem(resultItem, 51) + `]`, errorPart: "今日结果超过 50 条"},
		{name: "blockers", field: `"blockers":[]`, items: `"blockers":[` + repeatedJSONItem(blockerItem, 31) + `]`, errorPart: "死锁阻碍超过 30 条"},
		{name: "reviews", field: `"reviews":[]`, items: `"reviews":[` + repeatedJSONItem(reviewItem, 31) + `]`, errorPart: "专项复盘超过 30 条"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := strings.Replace(base, test.field, test.items, 1)
			if _, err := parseDailyReportGenerationResult(value, "2026-07-30"); err == nil ||
				!strings.Contains(err.Error(), test.errorPart) {
				t.Fatalf("expected %q error, got %v", test.errorPart, err)
			}
		})
	}

	tooMuchEvidence := `{"reportDate":"2026-07-30","results":[{"projectNo":"y15","projectName":"y15_app","task":"任务","status":"进行中","progress":"50%","evidence":[` + repeatedJSONItem(`"commit abc1234"`, 11) + `]}],"blockers":[],"reviews":[],"nextActions":[]}`
	if _, err := parseDailyReportGenerationResult(tooMuchEvidence, "2026-07-30"); err == nil ||
		!strings.Contains(err.Error(), "证据不能超过 10 条") {
		t.Fatalf("expected evidence limit error, got %v", err)
	}
}

func repeatedJSONItem(item string, count int) string {
	return strings.TrimSuffix(strings.Repeat(item+",", count), ",")
}

func TestDailyReportGeneratorRunsFakePIWithLockedDownArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI uses a POSIX shell")
	}
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "arguments.txt")
	promptCopyPath := filepath.Join(t.TempDir(), "prompt.txt")
	scriptPath := filepath.Join(binDir, "omp")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$BTASK_FAKE_ARGUMENTS"
for argument in "$@"; do
  case "$argument" in
    @*) cp "${argument#@}" "$BTASK_FAKE_PROMPT" ;;
  esac
done
printf '%s' '{"reportDate":"2026-07-30","results":[{"projectNo":"会议","projectName":"项目评审","task":"确认验收标准","status":"已完成","progress":"100%","evidence":["会议结论"]}],"blockers":[],"reviews":[],"nextActions":[{"projectNo":"y15","projectName":"y15_app","goal":"完成三个回归问题","deadline":"18:00 前","inferred":false}]}'
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake PI: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BTASK_FAKE_ARGUMENTS", argumentsPath)
	t.Setenv("BTASK_FAKE_PROMPT", promptCopyPath)

	input := dailyReportGenerationTestInput()
	var progressEvents []DailyReportGenerationProgress
	result, err := (DailyReportGenerator{}).Generate(
		context.Background(),
		input,
		PISettings{
			Model:          "openai-codex/gpt-test",
			ThinkingEffort: "high",
			TimeoutMinutes: 1,
		},
		func(progress DailyReportGenerationProgress) {
			progressEvents = append(progressEvents, progress)
		},
	)
	if err != nil {
		t.Fatalf("generate with fake PI: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Task != "确认验收标准" {
		t.Fatalf("unexpected generated result %#v", result)
	}
	expectedStages := []string{
		"validating",
		"collecting_git",
		"building_prompt",
		"waiting_ai",
		"parsing_result",
		"completed",
	}
	if len(progressEvents) != len(expectedStages) {
		t.Fatalf("unexpected progress events %#v", progressEvents)
	}
	for index, stage := range expectedStages {
		if progressEvents[index].RequestID != input.RequestID ||
			progressEvents[index].Stage != stage {
			t.Fatalf("unexpected progress event %d: %#v", index, progressEvents[index])
		}
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatalf("read fake PI arguments: %v", err)
	}
	for _, expected := range []string{
		"--no-tools",
		"--no-skills",
		"--no-rules",
		"--no-extensions",
		"--no-session",
		"--model=openai-codex/gpt-test",
		"--thinking=high",
		"--max-time=1m",
	} {
		if !strings.Contains(string(arguments), expected) {
			t.Fatalf("fake PI arguments missing %q:\n%s", expected, arguments)
		}
	}
	prompt, err := os.ReadFile(promptCopyPath)
	if err != nil {
		t.Fatalf("read fake PI prompt: %v", err)
	}
	if !strings.Contains(string(prompt), "参加项目评审") ||
		!strings.Contains(string(prompt), "固定规则") {
		t.Fatalf("unexpected PI prompt:\n%s", prompt)
	}
}

func TestDailyReportGeneratorRunsFakeCodexInEmptyReadOnlyWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI uses a POSIX shell")
	}
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "arguments.txt")
	promptCopyPath := filepath.Join(t.TempDir(), "prompt.txt")
	workspaceContentsPath := filepath.Join(t.TempDir(), "workspace.txt")
	scriptPath := filepath.Join(binDir, "codex")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$BTASK_FAKE_ARGUMENTS"
find . -mindepth 1 -maxdepth 1 -print > "$BTASK_FAKE_WORKSPACE"
cat > "$BTASK_FAKE_PROMPT"
printf '%s' '{"reportDate":"2026-07-30","results":[],"blockers":[],"reviews":[],"nextActions":[]}'
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake Codex: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BTASK_FAKE_ARGUMENTS", argumentsPath)
	t.Setenv("BTASK_FAKE_PROMPT", promptCopyPath)
	t.Setenv("BTASK_FAKE_WORKSPACE", workspaceContentsPath)

	input := dailyReportGenerationTestInput()
	input.Engine = "codex"
	result, err := (DailyReportGenerator{}).Generate(
		context.Background(),
		input,
		PISettings{
			Model:          "gpt-test",
			ThinkingEffort: "medium",
			TimeoutMinutes: 1,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("generate with fake Codex: %v", err)
	}
	if result.ReportDate != input.ReportDate || len(result.Results) != 0 {
		t.Fatalf("unexpected generated result %#v", result)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatalf("read fake Codex arguments: %v", err)
	}
	for _, expected := range []string{
		"exec",
		"--sandbox",
		"read-only",
		"--ephemeral",
		"--strict-config",
		"--ignore-user-config",
		"--ignore-rules",
		"--disable",
		"shell_tool",
		"unified_exec",
		"apps",
		"hooks",
		"--skip-git-repo-check",
		"--model",
		"gpt-test",
		`model_reasoning_effort="medium"`,
	} {
		if !strings.Contains(string(arguments), expected) {
			t.Fatalf("fake Codex arguments missing %q:\n%s", expected, arguments)
		}
	}
	workspaceContents, err := os.ReadFile(workspaceContentsPath)
	if err != nil {
		t.Fatalf("read fake Codex workspace: %v", err)
	}
	if strings.TrimSpace(string(workspaceContents)) != "" {
		t.Fatalf("Codex workspace was not empty: %s", workspaceContents)
	}
	prompt, err := os.ReadFile(promptCopyPath)
	if err != nil {
		t.Fatalf("read fake Codex prompt: %v", err)
	}
	if !strings.Contains(string(prompt), "参加项目评审") {
		t.Fatalf("unexpected Codex prompt:\n%s", prompt)
	}
}
