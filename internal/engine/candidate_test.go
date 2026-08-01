package engine

import "testing"

func TestParseCandidateAnalysis(t *testing.T) {
	analysis, err := parseCandidateAnalysis(`
说明：
{
  "title": "修复登录",
  "summaryMarkdown": "只处理来源中记录的登录失败。",
  "keyPoints": ["Android 可复现"],
  "openQuestions": ["iOS 是否受影响？"]
}`)
	if err != nil {
		t.Fatalf("parse candidate analysis: %v", err)
	}
	if analysis.Title != "修复登录" || len(analysis.OpenQuestions) != 1 {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
}

func TestParseCandidateAnalysisRejectsIncompleteOutput(t *testing.T) {
	if _, err := parseCandidateAnalysis(`{"title":"只有标题"}`); err == nil {
		t.Fatal("expected incomplete output to fail")
	}
}
