package engine

import (
	"os"
	"strings"
	"testing"
)

func requirementTestInput() RequirementAnalysisInput {
	return RequirementAnalysisInput{
		Task: RequirementTask{
			ID:                  "task-1",
			Title:               "需求访谈",
			OriginalDescription: "AI 发现缺口，用户作决定。",
		},
		Project: RequirementProject{Name: "BTaskAssistant"},
		Materials: []RequirementMaterial{
			{
				MaterialID: "source-1",
				Fragments: []RequirementMaterialFragment{
					{FragmentID: "source-1", Content: "需求必须人工批准。"},
				},
			},
		},
		Analyst: "pi",
		Round:   2,
		Mode:    "analyze",
	}
}

func TestPrepareRequirementAttachmentsExtractsImageData(t *testing.T) {
	input := requirementTestInput()
	input.Materials[0].Fragments[0].Content = "设计：![空状态](data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=)"

	prepared, attachments, cleanup, err := prepareRequirementAttachments(input)
	if err != nil {
		t.Fatalf("prepare attachments: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	content := prepared.Materials[0].Fragments[0].Content
	if strings.Contains(content, "base64") || !strings.Contains(content, "图片附件：空状态") {
		t.Fatalf("prepared content = %q", content)
	}
	if _, err := os.Stat(attachments[0].Path); err != nil {
		t.Fatalf("attachment file: %v", err)
	}
	path := attachments[0].Path
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("attachment was not cleaned up: %v", err)
	}
}

func TestBuildRequirementPromptUsesFixedInputAndReadOnlyRules(t *testing.T) {
	input := requirementTestInput()
	input.Project.LocalPath = t.TempDir()

	prompt, workDir, err := buildRequirementPrompt(input)
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if workDir != input.Project.LocalPath {
		t.Fatalf("work dir = %q, want %q", workDir, input.Project.LocalPath)
	}
	for _, expected := range []string{
		"固定输入包",
		"严禁创建、修改、删除文件",
		`"allowCodeWrite": false`,
		`"fragmentId": "source-1"`,
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q", expected)
		}
	}
}

func TestBuildRequirementPromptRequiresBoundProject(t *testing.T) {
	input := requirementTestInput()
	input.Project.Name = ""
	if _, _, err := buildRequirementPrompt(input); err == nil {
		t.Fatal("expected missing project to fail")
	}
}

func TestParseRequirementAnalysisFiltersUntraceableFactsAndCapsQuestions(t *testing.T) {
	output := `analysis follows
{
  "analysisId": "ANALYSIS-2",
  "confirmedFacts": [
    {"id":"FACT-1","content":"有来源事实","sourceFragmentIds":["source-1"]},
    {"id":"FACT-2","content":"无来源推测","sourceFragmentIds":["unknown"]}
  ],
  "projectObservations": [],
  "questions": [
    {"question":"问题一","reason":"影响范围","category":"scope","severity":"blocking","answerType":"text"},
    {"question":"问题二","reason":"影响行为","category":"behavior","severity":"important","answerType":"boolean"},
    {"question":"问题三","reason":"影响验收","category":"acceptance","severity":"optional","answerType":"text"},
    {"question":"问题四","reason":"影响约束","category":"constraint","severity":"important","answerType":"text"},
    {"question":"问题五","reason":"影响资料","category":"material","severity":"important","answerType":"file_or_image"},
    {"question":"问题六","reason":"不应进入结果","category":"other","severity":"optional","answerType":"text"}
  ],
  "conflicts": [],
  "draftUpdates": {},
  "analysisStatus": "needs_user_input",
  "recommendedAction": "ASK_QUESTIONS",
  "reason": "仍需回答"
}
trailing text`

	result, err := parseRequirementAnalysis(output, requirementTestInput())
	if err != nil {
		t.Fatalf("parse analysis: %v", err)
	}
	if len(result.ConfirmedFacts) != 1 {
		t.Fatalf("confirmed facts = %d, want 1", len(result.ConfirmedFacts))
	}
	if len(result.Questions) != 5 {
		t.Fatalf("questions = %d, want 5", len(result.Questions))
	}
	if result.Questions[0].Severity != "BLOCKING" {
		t.Fatalf("severity = %q, want BLOCKING", result.Questions[0].Severity)
	}
	if result.AnalysisStatus != "NEEDS_USER_INPUT" {
		t.Fatalf("status = %q, want NEEDS_USER_INPUT", result.AnalysisStatus)
	}
	if result.Round != 2 {
		t.Fatalf("round = %d, want 2", result.Round)
	}
}
