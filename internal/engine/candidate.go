package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/agent"
)

const maxCandidateSourceBytes = 512 * 1024

type CandidateAnalysis struct {
	Mode            string   `json:"mode"`
	Title           string   `json:"title"`
	SummaryMarkdown string   `json:"summaryMarkdown"`
	KeyPoints       []string `json:"keyPoints"`
	OpenQuestions   []string `json:"openQuestions"`
	AnalyzedAt      string   `json:"analyzedAt"`
}

func AnalyzePlaneCandidateWithPI(
	ctx context.Context,
	sourceMarkdown string,
	settings PISettings,
) (CandidateAnalysis, error) {
	sourceMarkdown = strings.TrimSpace(sourceMarkdown)
	if sourceMarkdown == "" {
		return CandidateAnalysis{}, errors.New("候选来源不能为空")
	}
	if len(sourceMarkdown) > maxCandidateSourceBytes {
		return CandidateAnalysis{}, errors.New("候选来源超过 512 KiB")
	}
	prompt := `你是 BTaskAssistant 的只读候选提炼器。
只能根据下方来源整理候选，不得虚构事实，不得执行工具或修改文件。
只返回一个 JSON object，不要 Markdown 代码块：
{"title":"简洁标题","summaryMarkdown":"完整候选正文","keyPoints":[],"openQuestions":[]}

来源：
` + sourceMarkdown
	output, err := runNativePIUtility(ctx, agent.UtilityRequest{
		Prompt: prompt, Model: settings.Model,
		ThinkingLevel: settings.ThinkingEffort, MaxOutputBytes: 1024 * 1024,
	})
	if err != nil {
		return CandidateAnalysis{}, fmt.Errorf("PI 候选提炼失败: %w", err)
	}
	analysis, err := parseCandidateAnalysis(output)
	if err != nil {
		return CandidateAnalysis{}, err
	}
	analysis.Mode = "pi"
	if strings.TrimSpace(analysis.AnalyzedAt) == "" {
		analysis.AnalyzedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return analysis, nil
}

func parseCandidateAnalysis(output string) (CandidateAnalysis, error) {
	value := strings.TrimSpace(output)
	if strings.HasPrefix(value, "```") {
		lines := strings.Split(value, "\n")
		if len(lines) >= 3 {
			value = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start < 0 || end <= start {
		return CandidateAnalysis{}, errors.New("PI 没有返回可识别的 JSON 候选")
	}
	var analysis CandidateAnalysis
	if err := json.Unmarshal([]byte(value[start:end+1]), &analysis); err != nil {
		return CandidateAnalysis{}, fmt.Errorf("PI 候选 JSON 无法解析: %w", err)
	}
	analysis.Title = strings.TrimSpace(analysis.Title)
	analysis.SummaryMarkdown = strings.TrimSpace(analysis.SummaryMarkdown)
	if analysis.Title == "" || analysis.SummaryMarkdown == "" {
		return CandidateAnalysis{}, errors.New("PI 候选缺少标题或正文")
	}
	if analysis.KeyPoints == nil {
		analysis.KeyPoints = []string{}
	}
	if analysis.OpenQuestions == nil {
		analysis.OpenQuestions = []string{}
	}
	return analysis, nil
}
