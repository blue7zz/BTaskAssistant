package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxOMPOutputBytes = 1024 * 1024

type CandidateAnalysis struct {
	Mode            string   `json:"mode"`
	Title           string   `json:"title"`
	SummaryMarkdown string   `json:"summaryMarkdown"`
	KeyPoints       []string `json:"keyPoints"`
	OpenQuestions   []string `json:"openQuestions"`
	AnalyzedAt      string   `json:"analyzedAt"`
}

type OMPAnalyzer struct {
	CommandPath string
	Settings    PISettings
}

func FindOMP() (string, error) {
	commandPath, err := exec.LookPath("omp")
	if err != nil {
		return "", errors.New("未找到 omp；请先安装并登录 oh-my-pi")
	}
	return commandPath, nil
}

func (a OMPAnalyzer) AnalyzeCandidate(
	ctx context.Context,
	sourceMarkdown string,
) (CandidateAnalysis, error) {
	sourceMarkdown = strings.TrimSpace(sourceMarkdown)
	if sourceMarkdown == "" {
		return CandidateAnalysis{}, errors.New("候选来源不能为空")
	}
	commandPath := a.CommandPath
	if commandPath == "" {
		var err error
		commandPath, err = FindOMP()
		if err != nil {
			return CandidateAnalysis{}, err
		}
	}

	prompt := `你是任务收集阶段的信息提炼器。只允许根据下面的 Plane 原始来源整理，不得假设、补造或扩展需求。

输出必须是一个 JSON 对象，不要使用 Markdown 代码围栏，也不要输出解释。字段固定为：
{
  "title": "简洁但不丢失原意的任务标题",
  "summaryMarkdown": "只基于来源整理的 Markdown 正文",
  "keyPoints": ["来源中明确写出的关键信息"],
  "openQuestions": ["来源缺失或含糊、必须交给用户确认的问题"]
}

如果来源没有某项信息，把它写入 openQuestions，不要自行回答。不要把你的判断写成事实。

Plane 原始来源：

` + sourceMarkdown

	args := []string{
		"--no-tools",
		"--no-skills",
		"--no-rules",
		"--no-extensions",
		"--no-title",
		"--no-session",
	}
	piArgs, err := piCommandArguments(a.Settings)
	if err != nil {
		return CandidateAnalysis{}, err
	}
	args = append(args, piArgs...)
	args = append(args, "--print", prompt)
	command := exec.CommandContext(ctx, commandPath, args...)
	command.Dir = os.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 300 {
			message = message[:300] + "…"
		}
		if message == "" {
			message = err.Error()
		}
		return CandidateAnalysis{}, fmt.Errorf("PI 提炼失败: %s", message)
	}
	if stdout.Len() > maxOMPOutputBytes {
		return CandidateAnalysis{}, errors.New("PI 返回内容过大，已拒绝保存")
	}
	analysis, err := parseCandidateAnalysis(stdout.String())
	if err != nil {
		return CandidateAnalysis{}, err
	}
	analysis.Mode = "pi"
	analysis.AnalyzedAt = time.Now().UTC().Format(time.RFC3339Nano)
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
