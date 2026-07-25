package plane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const maxPages = 50

var (
	htmlBreakPattern = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/h[1-6])\s*/?\s*>`)
	htmlTagPattern   = regexp.MustCompile(`<[^>]+>`)
)

type Candidate struct {
	ExternalID          string   `json:"externalId"`
	ExternalKey         string   `json:"externalKey"`
	Title               string   `json:"title"`
	DescriptionMarkdown string   `json:"descriptionMarkdown"`
	SourceMarkdown      string   `json:"sourceMarkdown"`
	Priority            string   `json:"priority"`
	StateName           string   `json:"stateName"`
	StateGroup          string   `json:"stateGroup"`
	Labels              []string `json:"labels"`
	Assignees           []string `json:"assignees"`
	CreatedAt           string   `json:"createdAt,omitempty"`
	UpdatedAt           string   `json:"updatedAt,omitempty"`
}

type ConnectionStatus struct {
	Connected bool   `json:"connected"`
	ItemCount int    `json:"itemCount"`
	Message   string `json:"message"`
}

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

type workItemEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type workItem struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       json.RawMessage  `json:"description"`
	DescriptionHTML   string           `json:"description_html"`
	Priority          string           `json:"priority"`
	SequenceID        int              `json:"sequence_id"`
	ProjectIdentifier string           `json:"project_identifier"`
	State             workItemEntity   `json:"state"`
	StateGroup        string           `json:"state_group"`
	Assignees         []workItemEntity `json:"assignees"`
	Labels            []workItemEntity `json:"labels"`
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
}

type listResponse struct {
	NextCursor      string     `json:"next_cursor"`
	NextPageResults bool       `json:"next_page_results"`
	TotalResults    int        `json:"total_results"`
	Results         []workItem `json:"results"`
}

func NormalizeBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("请输入 Plane 服务地址")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("Plane 服务地址格式不正确")
	}
	if parsed.User != nil {
		return "", errors.New("Plane 服务地址不能包含用户名或密码")
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", errors.New("PAT 只能发送到 HTTPS；HTTP 仅允许 localhost")
		}
	default:
		return "", errors.New("Plane 服务地址必须使用 HTTPS")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func NewClient(baseURL string, token string) (*Client, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("Plane Personal Access Token 尚未配置")
	}
	parsed, _ := url.Parse(normalized)
	return &Client{
		baseURL: parsed,
		token:   strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("Plane API 返回了重定向；已阻止转发 PAT")
			},
		},
	}, nil
}

func (c *Client) workItemsURL(workspaceSlug string, projectID string) (*url.URL, error) {
	workspaceSlug = strings.TrimSpace(workspaceSlug)
	projectID = strings.TrimSpace(projectID)
	if workspaceSlug == "" {
		return nil, errors.New("请输入 Plane workspace slug")
	}
	if projectID == "" {
		return nil, errors.New("请输入 Plane project ID")
	}
	result := *c.baseURL
	result.Path = path.Join(
		result.Path,
		"api",
		"v1",
		"workspaces",
		workspaceSlug,
		"projects",
		projectID,
		"work-items",
	) + "/"
	return &result, nil
}

func (c *Client) requestPage(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
	cursor string,
) (listResponse, error) {
	endpoint, err := c.workItemsURL(workspaceSlug, projectID)
	if err != nil {
		return listResponse{}, err
	}
	query := endpoint.Query()
	query.Set("per_page", "100")
	query.Set("order_by", "-updated_at")
	query.Set("expand", "state,assignees,labels")
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return listResponse{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-API-Key", c.token)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return listResponse{}, fmt.Errorf("连接 Plane 失败: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		return listResponse{}, fmt.Errorf("读取 Plane 响应失败: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if len(message) > 240 {
			message = message[:240] + "…"
		}
		return listResponse{}, fmt.Errorf(
			"Plane API 返回 %s: %s",
			response.Status,
			message,
		)
	}
	var result listResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return listResponse{}, fmt.Errorf("Plane 响应格式无法识别: %w", err)
	}
	return result, nil
}

func (c *Client) Test(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
) (ConnectionStatus, error) {
	page, err := c.requestPage(ctx, workspaceSlug, projectID, "")
	if err != nil {
		return ConnectionStatus{}, err
	}
	count := page.TotalResults
	if count == 0 {
		count = len(page.Results)
	}
	return ConnectionStatus{
		Connected: true,
		ItemCount: count,
		Message:   fmt.Sprintf("连接成功，项目中共有 %d 条工作项。", count),
	}, nil
}

func (c *Client) ListCandidates(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
) ([]Candidate, error) {
	var candidates []Candidate
	seen := make(map[string]struct{})
	cursor := ""
	for pageNumber := 0; pageNumber < maxPages; pageNumber++ {
		page, err := c.requestPage(ctx, workspaceSlug, projectID, cursor)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Results {
			if item.ID == "" {
				continue
			}
			if _, exists := seen[item.ID]; exists {
				continue
			}
			seen[item.ID] = struct{}{}
			candidates = append(candidates, normalizeWorkItem(item))
		}
		if !page.NextPageResults || page.NextCursor == "" {
			return candidates, nil
		}
		cursor = page.NextCursor
	}
	return nil, errors.New("Plane 工作项超过 5000 条，请缩小项目范围后重试")
}

func normalizeWorkItem(item workItem) Candidate {
	description := rawDescription(item.Description)
	if description == "" && item.DescriptionHTML != "" {
		description = htmlToMarkdownText(item.DescriptionHTML)
	}
	if description == "" {
		description = "（Plane 中未填写描述）"
	}
	labels := entityNames(item.Labels)
	assignees := entityNames(item.Assignees)
	stateName := strings.TrimSpace(item.State.Name)
	stateGroup := strings.TrimSpace(item.StateGroup)
	externalKey := fmt.Sprintf("#%d", item.SequenceID)
	if item.ProjectIdentifier != "" {
		externalKey = fmt.Sprintf("%s-%d", item.ProjectIdentifier, item.SequenceID)
	}

	source := strings.Builder{}
	fmt.Fprintf(&source, "# %s\n\n", strings.TrimSpace(item.Name))
	fmt.Fprintf(&source, "- Plane ID: `%s`\n", item.ID)
	fmt.Fprintf(&source, "- 编号: `%s`\n", externalKey)
	fmt.Fprintf(&source, "- 状态: %s\n", fallback(stateName, "未设置"))
	fmt.Fprintf(&source, "- 优先级: %s\n", fallback(item.Priority, "未设置"))
	fmt.Fprintf(&source, "- 标签: %s\n", fallback(strings.Join(labels, "、"), "无"))
	fmt.Fprintf(&source, "- 负责人: %s\n", fallback(strings.Join(assignees, "、"), "无"))
	if item.CreatedAt != "" {
		fmt.Fprintf(&source, "- 创建时间: %s\n", item.CreatedAt)
	}
	if item.UpdatedAt != "" {
		fmt.Fprintf(&source, "- 更新时间: %s\n", item.UpdatedAt)
	}
	fmt.Fprintf(&source, "\n## Plane 原始描述\n\n%s\n", description)

	return Candidate{
		ExternalID:          item.ID,
		ExternalKey:         externalKey,
		Title:               strings.TrimSpace(item.Name),
		DescriptionMarkdown: description,
		SourceMarkdown:      source.String(),
		Priority:            normalizePriority(item.Priority),
		StateName:           stateName,
		StateGroup:          stateGroup,
		Labels:              labels,
		Assignees:           assignees,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
	}
}

func rawDescription(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if strings.Contains(text, "<") {
			return htmlToMarkdownText(text)
		}
		return strings.TrimSpace(text)
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	pretty, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return "```json\n" + string(pretty) + "\n```"
}

func htmlToMarkdownText(value string) string {
	value = htmlBreakPattern.ReplaceAllString(value, "\n")
	value = htmlTagPattern.ReplaceAllString(value, "")
	lines := strings.Split(html.UnescapeString(value), "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n\n")
}

func entityNames(items []workItemEntity) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		name := fallback(
			strings.TrimSpace(item.Name),
			strings.TrimSpace(item.DisplayName),
		)
		name = fallback(name, strings.TrimSpace(item.Email))
		name = fallback(name, strings.TrimSpace(item.ID))
		if name != "" {
			result = append(result, name)
		}
	}
	return result
}

func normalizePriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "urgent", "high":
		return "high"
	case "low":
		return "low"
	default:
		return "medium"
	}
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}
