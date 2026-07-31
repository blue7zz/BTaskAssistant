package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrPIUtilityRPCUnavailable = errors.New(
	"原生 PI RPC 将在阶段 2 接入；当前不会调用 OMP，请改用 Codex 或人工整理",
)

type CandidateAnalysis struct {
	Mode            string   `json:"mode"`
	Title           string   `json:"title"`
	SummaryMarkdown string   `json:"summaryMarkdown"`
	KeyPoints       []string `json:"keyPoints"`
	OpenQuestions   []string `json:"openQuestions"`
	AnalyzedAt      string   `json:"analyzedAt"`
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
