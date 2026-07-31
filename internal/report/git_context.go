package report

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	gitURLPattern = regexp.MustCompile(
		`(?i)\b(?:https?|ssh|git)://[^[:space:]<>"']+`,
	)
	gitSCPRemotePattern = regexp.MustCompile(
		`\b[^@[:space:]]+@[A-Za-z0-9.\-]+:[^[:space:]<>"']+`,
	)
	gitEmailPattern = regexp.MustCompile(
		`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`,
	)
	gitUnixAbsolutePathPattern = regexp.MustCompile(
		`(^|[[:space:]\(\[\{"'=,:;>])\/[^[:space:]\)\]\}"',;]+`,
	)
	gitWindowsAbsolutePathPattern = regexp.MustCompile(
		`(?i)[A-Z]:[\\/][^[:space:]\)\]\}"',;]*`,
	)
	gitUNCAbsolutePathPattern = regexp.MustCompile(
		`\\\\[^\\[:space:]]+\\[^[:space:]\)\]\}"',;]+`,
	)
)

const (
	maxGitProjects      = 8
	maxGitCommits       = 100
	maxGitCommandOutput = 256 << 10
	maxGitProjectOutput = 256 << 10
	maxGitContextOutput = 2 << 20
	maxGitProjectField  = 512
	gitCommitStatMarker = "__BTA_COMMIT__"
	gitReportDateLayout = "2006-01-02"
)

// GitProject identifies a repository without exposing its local path to the
// generated context. Path is used only as the working directory for Git.
type GitProject struct {
	ProjectNo   string
	ProjectName string
	Path        string
}

type gitCommit struct {
	hash    string
	time    string
	subject string
	stat    string
}

// CollectGitContext collects a bounded, read-only summary for daily-report AI
// input. It never invokes a shell and does not read remotes, author email, or
// diff contents.
func CollectGitContext(
	ctx context.Context,
	reportDate string,
	gitAuthor string,
	includeUncommitted bool,
	projects []GitProject,
) (string, error) {
	reportDate = strings.TrimSpace(reportDate)
	date, err := time.Parse(gitReportDateLayout, reportDate)
	if err != nil || date.Format(gitReportDateLayout) != reportDate {
		return "", errors.New("日报日期必须使用 YYYY-MM-DD 格式")
	}
	if len(projects) == 0 {
		return "No Git projects selected.", nil
	}
	if len(projects) > maxGitProjects {
		return "", fmt.Errorf("Git 项目不能超过 %d 个", maxGitProjects)
	}
	gitAuthor = strings.TrimSpace(gitAuthor)
	if len(gitAuthor) > maxGitProjectField {
		return "", errors.New("Git 作者过滤条件过长")
	}

	var result bytes.Buffer
	for index, project := range projects {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		projectContext, err := collectGitProject(
			ctx,
			reportDate,
			date.AddDate(0, 0, 1).Format(gitReportDateLayout),
			gitAuthor,
			includeUncommitted,
			project,
			index,
		)
		if err != nil {
			return "", err
		}
		separatorSize := 0
		if result.Len() > 0 {
			separatorSize = 1
		}
		if result.Len()+separatorSize+len(projectContext) > maxGitContextOutput {
			return "", errors.New("Git 上下文总输出过大")
		}
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(projectContext)
	}
	return result.String(), nil
}

func collectGitProject(
	ctx context.Context,
	startDate string,
	endDate string,
	gitAuthor string,
	includeUncommitted bool,
	project GitProject,
	index int,
) (string, error) {
	project.ProjectNo = cleanGitLine(project.ProjectNo)
	project.ProjectName = cleanGitLine(project.ProjectName)
	project.Path = strings.TrimSpace(project.Path)
	label := gitProjectLabel(project, index)
	if project.Path == "" {
		return "", fmt.Errorf("%s 缺少项目路径", label)
	}
	if len(project.ProjectNo) > maxGitProjectField ||
		len(project.ProjectName) > maxGitProjectField {
		return "", fmt.Errorf("%s 的项目编号或名称过长", label)
	}
	info, err := os.Stat(project.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s 的项目路径不存在", label)
		}
		return "", fmt.Errorf("无法读取 %s 的项目路径", label)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s 的项目路径不是目录", label)
	}

	inside, err := runGit(ctx, project.Path, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return "", fmt.Errorf("%s 不是 Git 仓库", label)
	}

	branch, err := gitBranch(ctx, project.Path)
	if err != nil {
		return "", fmt.Errorf("读取 %s 的分支失败", label)
	}
	commits, err := gitCommits(
		ctx,
		project.Path,
		startDate,
		endDate,
		gitAuthorForProject(ctx, project.Path, gitAuthor),
	)
	if err != nil {
		return "", fmt.Errorf("读取 %s 的提交记录失败: %w", label, err)
	}

	var output bytes.Buffer
	output.WriteString("PROJECT\n")
	fmt.Fprintf(&output, "project_no: %s\n", valueOrNone(project.ProjectNo))
	fmt.Fprintf(&output, "project_name: %s\n", valueOrNone(project.ProjectName))
	fmt.Fprintf(&output, "branch: %s\n", cleanGitLine(branch))
	output.WriteString("commits:\n")
	if len(commits) == 0 {
		output.WriteString("- none\n")
	} else {
		for _, commit := range commits {
			fmt.Fprintf(
				&output,
				"- hash: %s\n  time: %s\n  subject: %s\n  stat: %s\n",
				shortGitHash(commit.hash),
				cleanGitLine(commit.time),
				cleanGitLine(commit.subject),
				valueOrNone(cleanGitLine(commit.stat)),
			)
		}
	}

	if includeUncommitted {
		status, err := runGit(
			ctx,
			project.Path,
			"status",
			"--porcelain=v1",
			"--untracked-files=normal",
		)
		if err != nil {
			return "", fmt.Errorf("读取 %s 的未提交状态失败", label)
		}
		workingStat, err := runGit(
			ctx,
			project.Path,
			"diff",
			"--no-ext-diff",
			"--no-textconv",
			"--no-renames",
			"--stat",
			"--",
		)
		if err != nil {
			return "", fmt.Errorf("读取 %s 的工作区统计失败", label)
		}
		stagedStat, err := runGit(
			ctx,
			project.Path,
			"diff",
			"--cached",
			"--no-ext-diff",
			"--no-textconv",
			"--no-renames",
			"--stat",
			"--",
		)
		if err != nil {
			return "", fmt.Errorf("读取 %s 的暂存区统计失败", label)
		}

		output.WriteString("uncommitted:\n")
		writeGitLines(&output, "status", status)
		writeGitLines(&output, "working_tree_stat", workingStat)
		writeGitLines(&output, "staged_stat", stagedStat)
	}

	value := strings.ReplaceAll(output.String(), project.Path, "[redacted-path]")
	if len(value) > maxGitProjectOutput {
		return "", fmt.Errorf("%s 的 Git 上下文输出过大", label)
	}
	return value, nil
}

func gitBranch(ctx context.Context, repoPath string) (string, error) {
	branch, err := runGit(ctx, repoPath, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		return strings.TrimSpace(branch), nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	hash, hashErr := runGit(ctx, repoPath, "rev-parse", "--short=12", "HEAD")
	if hashErr == nil {
		return "detached@" + strings.TrimSpace(hash), nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	return "unborn", nil
}

func gitCommits(
	ctx context.Context,
	repoPath string,
	startDate string,
	endDate string,
	gitAuthor string,
) ([]gitCommit, error) {
	if _, err := runGit(ctx, repoPath, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, nil
	}

	args := []string{
		"log",
		"--no-ext-diff",
		"--no-textconv",
		"--no-merges",
		"--max-count=" + fmt.Sprint(maxGitCommits),
		"--since=" + startDate + " 00:00:00",
		"--before=" + endDate + " 00:00:00",
		"--format=%H%x09%cI%x09%s",
	}
	if gitAuthor != "" {
		args = append(args, "--author="+gitAuthor)
	}
	logOutput, err := runGit(ctx, repoPath, args...)
	if err != nil {
		return nil, err
	}

	var commits []gitCommit
	var hashes []string
	for _, line := range strings.Split(strings.TrimSpace(logOutput), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			return nil, errors.New("Git 返回了无法解析的提交记录")
		}
		commits = append(commits, gitCommit{
			hash:    fields[0],
			time:    fields[1],
			subject: fields[2],
		})
		hashes = append(hashes, fields[0])
	}
	if len(commits) == 0 {
		return nil, nil
	}

	statArgs := []string{
		"show",
		"--no-ext-diff",
		"--no-textconv",
		"--no-renames",
		"--format=" + gitCommitStatMarker + "%H",
		"--shortstat",
	}
	statArgs = append(statArgs, hashes...)
	statArgs = append(statArgs, "--")
	statOutput, err := runGit(ctx, repoPath, statArgs...)
	if err != nil {
		return nil, err
	}
	stats := parseGitCommitStats(statOutput)
	for index := range commits {
		commits[index].stat = stats[commits[index].hash]
	}
	return commits, nil
}

func gitAuthorForProject(
	ctx context.Context,
	repoPath string,
	configured string,
) string {
	if configured != "" {
		return configured
	}
	value, err := runGit(ctx, repoPath, "config", "--get", "user.name")
	if err != nil {
		return ""
	}
	return cleanGitLine(value)
}

func parseGitCommitStats(output string) map[string]string {
	stats := make(map[string]string)
	currentHash := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, gitCommitStatMarker) {
			currentHash = strings.TrimPrefix(line, gitCommitStatMarker)
			stats[currentHash] = "none"
			continue
		}
		if currentHash != "" && line != "" {
			stats[currentHash] = cleanGitLine(line)
		}
	}
	return stats
}

func writeGitLines(output *bytes.Buffer, heading string, value string) {
	fmt.Fprintf(output, "  %s:\n", heading)
	wroteLine := false
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		line = cleanGitLine(line)
		if line == "" {
			continue
		}
		fmt.Fprintf(output, "  - %s\n", line)
		wroteLine = true
	}
	if !wroteLine {
		output.WriteString("  - none\n")
	}
}

func runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	gitArgs := []string{
		"-C", repoPath,
		"-c", "color.ui=false",
		"-c", "core.pager=cat",
		"-c", "core.fsmonitor=false",
		"-c", "core.quotePath=true",
	}
	gitArgs = append(gitArgs, args...)
	command := exec.CommandContext(ctx, "git", gitArgs...)
	command.Env = append(
		os.Environ(),
		"LC_ALL=C",
		"LANG=C",
		"GIT_OPTIONAL_LOCKS=0",
	)
	stdout := &boundedGitBuffer{limit: maxGitCommandOutput}
	stderr := &boundedGitBuffer{limit: 8 << 10}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if stdout.exceeded {
		return "", errors.New("Git 命令输出过大")
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", errors.New("Git 命令执行失败")
	}
	return stdout.String(), nil
}

type boundedGitBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (buffer *boundedGitBuffer) Write(value []byte) (int, error) {
	if buffer.exceeded {
		return len(value), nil
	}
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.exceeded = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = buffer.buffer.Write(value[:remaining])
		buffer.exceeded = true
		return len(value), nil
	}
	_, _ = buffer.buffer.Write(value)
	return len(value), nil
}

func (buffer *boundedGitBuffer) String() string {
	return buffer.buffer.String()
}

func cleanGitLine(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	value = gitURLPattern.ReplaceAllString(value, "[redacted-url]")
	value = gitSCPRemotePattern.ReplaceAllString(value, "[redacted-remote]")
	value = gitEmailPattern.ReplaceAllString(value, "[redacted-email]")
	value = gitUNCAbsolutePathPattern.ReplaceAllString(value, "[redacted-path]")
	value = gitWindowsAbsolutePathPattern.ReplaceAllString(
		value,
		"[redacted-path]",
	)
	value = gitUnixAbsolutePathPattern.ReplaceAllString(
		value,
		"$1[redacted-path]",
	)
	return value
}

func shortGitHash(hash string) string {
	hash = cleanGitLine(hash)
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func gitProjectLabel(project GitProject, index int) string {
	if project.ProjectNo != "" {
		return "项目 " + project.ProjectNo
	}
	if project.ProjectName != "" {
		return "项目 " + project.ProjectName
	}
	return fmt.Sprintf("第 %d 个项目", index+1)
}

func valueOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
