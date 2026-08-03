package plane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	xhtml "golang.org/x/net/html"
)

const maxPages = 50

type Candidate struct {
	ExternalID          string              `json:"externalId"`
	ExternalKey         string              `json:"externalKey"`
	Title               string              `json:"title"`
	DescriptionMarkdown string              `json:"descriptionMarkdown"`
	SourceMarkdown      string              `json:"sourceMarkdown"`
	Priority            string              `json:"priority"`
	StateName           string              `json:"stateName"`
	StateGroup          string              `json:"stateGroup"`
	Labels              []string            `json:"labels"`
	Assignees           []string            `json:"assignees"`
	AssigneeDetails     []Person            `json:"assigneeDetails"`
	Comments            []Comment           `json:"comments"`
	CommentsSyncError   string              `json:"commentsSyncError,omitempty"`
	DetailsLoaded       bool                `json:"detailsLoaded"`
	Parent              *CandidateReference `json:"parent,omitempty"`
	CreatedAt           string              `json:"createdAt,omitempty"`
	UpdatedAt           string              `json:"updatedAt,omitempty"`
}

type CandidateReference struct {
	ExternalID  string `json:"externalId"`
	ExternalKey string `json:"externalKey"`
	Title       string `json:"title"`
}

type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Comment struct {
	ID           string `json:"id"`
	BodyMarkdown string `json:"bodyMarkdown"`
	Actor        Person `json:"actor"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
	EditedAt     string `json:"editedAt,omitempty"`
}

type ConnectionStatus struct {
	Connected bool   `json:"connected"`
	ItemCount int    `json:"itemCount"`
	Message   string `json:"message"`
}

type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

type ConnectionSetup struct {
	BaseURL       string    `json:"baseUrl"`
	WorkspaceSlug string    `json:"workspaceSlug"`
	Projects      []Project `json:"projects"`
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
	Group       string `json:"group"`
}

type workItem struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       json.RawMessage  `json:"description"`
	DescriptionHTML   string           `json:"description_html"`
	Priority          string           `json:"priority"`
	SequenceID        int              `json:"sequence_id"`
	ProjectIdentifier string           `json:"project_identifier"`
	Parent            json.RawMessage  `json:"parent"`
	ParentID          string           `json:"parent_id"`
	State             workItemEntity   `json:"state"`
	StateGroup        string           `json:"state_group"`
	Assignees         []workItemEntity `json:"assignees"`
	Labels            []workItemEntity `json:"labels"`
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
}

type workItemComment struct {
	ID              string          `json:"id"`
	CommentStripped string          `json:"comment_stripped"`
	CommentHTML     string          `json:"comment_html"`
	Actor           json.RawMessage `json:"actor"`
	ActorDetail     workItemEntity  `json:"actor_detail"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
	EditedAt        string          `json:"edited_at"`
}

type listResponse struct {
	NextCursor      string     `json:"next_cursor"`
	NextPageResults bool       `json:"next_page_results"`
	TotalResults    int        `json:"total_results"`
	Results         []workItem `json:"results"`
}

type commentListResponse struct {
	NextCursor      string            `json:"next_cursor"`
	NextPageResults bool              `json:"next_page_results"`
	Results         []workItemComment `json:"results"`
}

type projectListResponse struct {
	Results []Project `json:"results"`
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

// ResolveWorkspaceURL accepts a URL copied from the Plane web app, such as
// https://plane.example.com/my-team/ or a deeper project URL. Plane's public
// REST API cannot enumerate workspaces from a PAT, so the workspace slug is
// deliberately derived from the visible web URL instead of asking the user to
// type an internal slug field.
func ResolveWorkspaceURL(raw string) (ConnectionSetup, error) {
	normalized, err := NormalizeBaseURL(raw)
	if err != nil {
		return ConnectionSetup{}, err
	}
	parsed, _ := url.Parse(normalized)
	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool {
		return r == '/'
	})
	if len(segments) == 0 {
		return ConnectionSetup{}, errors.New(
			"请粘贴 Plane 工作区地址，例如 https://plane.example.com/my-team/",
		)
	}
	workspaceSlug := strings.TrimSpace(segments[0])
	switch strings.ToLower(workspaceSlug) {
	case "api", "auth", "god-mode", "profile", "settings":
		return ConnectionSetup{}, errors.New(
			"服务地址中没有识别到工作区，请粘贴浏览器里的 Plane 工作区页面地址",
		)
	}

	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if strings.EqualFold(parsed.Hostname(), "app.plane.so") {
		parsed.Host = "api.plane.so"
	}
	return ConnectionSetup{
		BaseURL:       strings.TrimRight(parsed.String(), "/"),
		WorkspaceSlug: workspaceSlug,
	}, nil
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

func (c *Client) commentsURL(
	workspaceSlug string,
	projectID string,
	workItemID string,
) (*url.URL, error) {
	endpoint, err := c.workItemsURL(workspaceSlug, projectID)
	if err != nil {
		return nil, err
	}
	workItemID = strings.TrimSpace(workItemID)
	if workItemID == "" {
		return nil, errors.New("Plane 工作项 ID 不能为空")
	}
	endpoint.Path = path.Join(endpoint.Path, workItemID, "comments") + "/"
	return endpoint, nil
}

func (c *Client) workItemURL(
	workspaceSlug string,
	projectID string,
	workItemID string,
) (*url.URL, error) {
	endpoint, err := c.workItemsURL(workspaceSlug, projectID)
	if err != nil {
		return nil, err
	}
	workItemID = strings.TrimSpace(workItemID)
	if workItemID == "" {
		return nil, errors.New("Plane 工作项 ID 不能为空")
	}
	endpoint.Path = path.Join(endpoint.Path, workItemID) + "/"
	return endpoint, nil
}

func (c *Client) projectsURL(workspaceSlug string) (*url.URL, error) {
	workspaceSlug = strings.TrimSpace(workspaceSlug)
	if workspaceSlug == "" {
		return nil, errors.New("请输入 Plane workspace slug")
	}
	result := *c.baseURL
	result.Path = path.Join(
		result.Path,
		"api",
		"v1",
		"workspaces",
		workspaceSlug,
		"projects",
	) + "/"
	return &result, nil
}

func (c *Client) authorizedGET(
	ctx context.Context,
	endpoint *url.URL,
) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-API-Key", c.token)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接 Plane 失败: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("读取 Plane 响应失败: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if len(message) > 240 {
			message = message[:240] + "…"
		}
		return nil, fmt.Errorf(
			"Plane API 返回 %s: %s",
			response.Status,
			message,
		)
	}
	return body, nil
}

func (c *Client) ListProjects(
	ctx context.Context,
	workspaceSlug string,
) ([]Project, error) {
	endpoint, err := c.projectsURL(workspaceSlug)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("per_page", "100")
	query.Set("order_by", "name")
	endpoint.RawQuery = query.Encode()

	body, err := c.authorizedGET(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var envelope projectListResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		var projects []Project
		if arrayErr := json.Unmarshal(body, &projects); arrayErr != nil {
			return nil, fmt.Errorf("Plane 项目列表格式无法识别: %w", err)
		}
		envelope.Results = projects
	}
	projects := make([]Project, 0, len(envelope.Results))
	seen := make(map[string]struct{})
	for _, project := range envelope.Results {
		project.ID = strings.TrimSpace(project.ID)
		project.Name = strings.TrimSpace(project.Name)
		project.Identifier = strings.TrimSpace(project.Identifier)
		if project.ID == "" {
			continue
		}
		if _, exists := seen[project.ID]; exists {
			continue
		}
		seen[project.ID] = struct{}{}
		projects = append(projects, project)
	}
	sort.SliceStable(projects, func(left int, right int) bool {
		return strings.ToLower(projects[left].Name) <
			strings.ToLower(projects[right].Name)
	})
	return projects, nil
}

func (c *Client) requestPage(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
	cursor string,
	assigneeID string,
) (listResponse, error) {
	endpoint, err := c.workItemsURL(workspaceSlug, projectID)
	if err != nil {
		return listResponse{}, err
	}
	query := endpoint.Query()
	query.Set("per_page", "100")
	query.Set("order_by", "-updated_at")
	query.Set(
		"fields",
		"id,name,priority,sequence_id,state,assignees,labels,created_at,updated_at",
	)
	query.Set("expand", "state,assignees,labels")
	if assigneeID = strings.TrimSpace(assigneeID); assigneeID != "" {
		query.Set("assignee", assigneeID)
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()

	body, err := c.authorizedGET(ctx, endpoint)
	if err != nil {
		return listResponse{}, err
	}
	var result listResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return listResponse{}, fmt.Errorf("Plane 响应格式无法识别: %w", err)
	}
	return result, nil
}

func (c *Client) requestWorkItem(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
	workItemID string,
) (workItem, error) {
	endpoint, err := c.workItemURL(workspaceSlug, projectID, workItemID)
	if err != nil {
		return workItem{}, err
	}
	query := endpoint.Query()
	query.Set("expand", "state,assignees,labels")
	endpoint.RawQuery = query.Encode()

	body, err := c.authorizedGET(ctx, endpoint)
	if err != nil {
		return workItem{}, err
	}
	var result workItem
	if err := json.Unmarshal(body, &result); err != nil {
		return workItem{}, fmt.Errorf("Plane 工作项详情格式无法识别: %w", err)
	}
	if strings.TrimSpace(result.ID) == "" {
		return workItem{}, errors.New("Plane 工作项详情缺少 ID")
	}
	if strings.TrimSpace(result.ID) != strings.TrimSpace(workItemID) {
		return workItem{}, fmt.Errorf(
			"Plane 工作项详情 ID 不匹配: 请求 %q，返回 %q",
			strings.TrimSpace(workItemID),
			strings.TrimSpace(result.ID),
		)
	}
	return result, nil
}

func (c *Client) requestCommentsPage(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
	workItemID string,
	cursor string,
) (commentListResponse, error) {
	endpoint, err := c.commentsURL(workspaceSlug, projectID, workItemID)
	if err != nil {
		return commentListResponse{}, err
	}
	query := endpoint.Query()
	query.Set("per_page", "100")
	query.Set("order_by", "created_at")
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()

	body, err := c.authorizedGET(ctx, endpoint)
	if err != nil {
		return commentListResponse{}, err
	}
	var result commentListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		var comments []workItemComment
		if arrayErr := json.Unmarshal(body, &comments); arrayErr != nil {
			return commentListResponse{}, fmt.Errorf(
				"Plane 评论响应格式无法识别: %w",
				err,
			)
		}
		result.Results = comments
	}
	return result, nil
}

func (c *Client) listComments(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
	workItemID string,
) ([]Comment, error) {
	comments := make([]Comment, 0)
	seen := make(map[string]struct{})
	cursor := ""
	for pageNumber := 0; pageNumber < maxPages; pageNumber++ {
		page, err := c.requestCommentsPage(
			ctx,
			workspaceSlug,
			projectID,
			workItemID,
			cursor,
		)
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
			comment := normalizeComment(item)
			if comment.BodyMarkdown == "" {
				continue
			}
			comments = append(comments, comment)
		}
		if !page.NextPageResults || page.NextCursor == "" {
			return comments, nil
		}
		cursor = page.NextCursor
	}
	return nil, errors.New("Plane 单条工作项的评论超过 5000 条")
}

func (c *Client) Test(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
) (ConnectionStatus, error) {
	page, err := c.requestPage(ctx, workspaceSlug, projectID, "", "")
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
	projectIdentifier string,
	assigneeIDs []string,
) ([]Candidate, error) {
	var items []workItem
	seen := make(map[string]struct{})
	filters := make([]string, 0, len(assigneeIDs))
	seenFilters := make(map[string]struct{}, len(assigneeIDs))
	for _, assigneeID := range assigneeIDs {
		assigneeID = strings.TrimSpace(assigneeID)
		if assigneeID == "" {
			continue
		}
		if _, exists := seenFilters[assigneeID]; exists {
			continue
		}
		seenFilters[assigneeID] = struct{}{}
		filters = append(filters, assigneeID)
	}
	if len(filters) == 0 {
		filters = append(filters, "")
	}

	for _, assigneeID := range filters {
		cursor := ""
		finished := false
		for pageNumber := 0; pageNumber < maxPages; pageNumber++ {
			page, err := c.requestPage(
				ctx,
				workspaceSlug,
				projectID,
				cursor,
				assigneeID,
			)
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
				items = append(items, item)
			}
			if !page.NextPageResults || page.NextCursor == "" {
				finished = true
				break
			}
			cursor = page.NextCursor
		}
		if !finished {
			return nil, errors.New("Plane 工作项超过 5000 条，请缩小项目范围后重试")
		}
	}

	candidates := make([]Candidate, len(items))
	for index, item := range items {
		candidates[index] = normalizeWorkItem(
			item,
			projectIdentifier,
			false,
		)
	}
	return candidates, nil
}

func (c *Client) LoadCandidateDetails(
	ctx context.Context,
	workspaceSlug string,
	projectID string,
	projectIdentifier string,
	workItemID string,
) (Candidate, error) {
	item, err := c.requestWorkItem(ctx, workspaceSlug, projectID, workItemID)
	if err != nil {
		return Candidate{}, err
	}
	candidate := normalizeWorkItem(item, projectIdentifier, true)
	if parentID := item.parentWorkItemID(); parentID != "" && parentID != item.ID {
		parentItem, parentErr := c.requestWorkItem(
			ctx,
			workspaceSlug,
			projectID,
			parentID,
		)
		if parentErr == nil {
			parent := normalizeCandidateReference(parentItem, projectIdentifier)
			candidate.Parent = &parent
		}
	}
	comments, err := c.listComments(
		ctx,
		workspaceSlug,
		projectID,
		candidate.ExternalID,
	)
	if err != nil {
		candidate.CommentsSyncError = friendlyCommentError(err)
	} else {
		candidate.Comments = comments
	}
	candidate.SourceMarkdown = appendCommentsToSource(
		candidate.SourceMarkdown,
		candidate.Comments,
		candidate.CommentsSyncError,
	)
	return candidate, nil
}

func normalizeWorkItem(
	item workItem,
	projectIdentifier string,
	includeDescription bool,
) Candidate {
	description := ""
	if includeDescription {
		if strings.TrimSpace(item.DescriptionHTML) != "" {
			description = htmlToMarkdown(item.DescriptionHTML)
		}
		if description == "" {
			description = rawDescription(item.Description)
		}
		if description == "" {
			description = "（Plane 中未填写描述）"
		}
	}
	labels := entityNames(item.Labels)
	assigneeDetails := entityPeople(item.Assignees)
	assignees := personNames(assigneeDetails)
	stateName := strings.TrimSpace(item.State.Name)
	stateGroup := fallback(
		strings.TrimSpace(item.StateGroup),
		strings.TrimSpace(item.State.Group),
	)
	externalKey := workItemExternalKey(item, projectIdentifier)

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
	if includeDescription {
		fmt.Fprintf(&source, "\n## Plane 原始描述\n\n%s\n", description)
	}

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
		AssigneeDetails:     assigneeDetails,
		Comments:            []Comment{},
		DetailsLoaded:       includeDescription,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
	}
}

func (item workItem) parentWorkItemID() string {
	if parentID := strings.TrimSpace(item.ParentID); parentID != "" {
		return parentID
	}
	if len(item.Parent) == 0 || string(item.Parent) == "null" {
		return ""
	}
	var parentID string
	if json.Unmarshal(item.Parent, &parentID) == nil {
		return strings.TrimSpace(parentID)
	}
	var parent struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(item.Parent, &parent) == nil {
		return strings.TrimSpace(parent.ID)
	}
	return ""
}

func normalizeCandidateReference(
	item workItem,
	projectIdentifier string,
) CandidateReference {
	return CandidateReference{
		ExternalID:  strings.TrimSpace(item.ID),
		ExternalKey: workItemExternalKey(item, projectIdentifier),
		Title:       strings.TrimSpace(item.Name),
	}
}

func workItemExternalKey(item workItem, projectIdentifier string) string {
	projectIdentifier = fallback(
		strings.TrimSpace(item.ProjectIdentifier),
		strings.TrimSpace(projectIdentifier),
	)
	if projectIdentifier == "" {
		return fmt.Sprintf("#%d", item.SequenceID)
	}
	return fmt.Sprintf("%s-%d", projectIdentifier, item.SequenceID)
}

func normalizeComment(item workItemComment) Comment {
	body := strings.TrimSpace(item.CommentStripped)
	if body == "" {
		body = htmlToMarkdown(item.CommentHTML)
	}
	actor := entityPerson(item.ActorDetail)
	if actor.Name == "" && len(item.Actor) > 0 {
		var expanded workItemEntity
		if json.Unmarshal(item.Actor, &expanded) == nil {
			actor = entityPerson(expanded)
		}
	}
	if actor.Name == "" {
		actor = Person{
			ID:   strings.TrimSpace(item.CreatedBy),
			Name: "Plane 用户",
		}
	}
	return Comment{
		ID:           strings.TrimSpace(item.ID),
		BodyMarkdown: body,
		Actor:        actor,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
		EditedAt:     item.EditedAt,
	}
}

func appendCommentsToSource(
	source string,
	comments []Comment,
	syncError string,
) string {
	var result strings.Builder
	result.WriteString(strings.TrimRight(source, "\n"))
	result.WriteString("\n\n## Plane 评论\n\n")
	if syncError != "" {
		fmt.Fprintf(&result, "> %s\n", syncError)
		return result.String()
	}
	if len(comments) == 0 {
		result.WriteString("暂无评论。\n")
		return result.String()
	}
	for _, comment := range comments {
		actorName := fallback(comment.Actor.Name, "Plane 用户")
		timestamp := fallback(comment.CreatedAt, "时间未知")
		fmt.Fprintf(&result, "### %s · %s\n\n", actorName, timestamp)
		result.WriteString(strings.TrimSpace(comment.BodyMarkdown))
		result.WriteString("\n\n")
	}
	return strings.TrimRight(result.String(), "\n") + "\n"
}

func friendlyCommentError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 180 {
		message = message[:180] + "…"
	}
	return "评论同步失败，可稍后重试：" + message
}

func rawDescription(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if strings.Contains(text, "<") {
			return htmlToMarkdown(text)
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

func htmlToMarkdown(value string) string {
	document, err := xhtml.Parse(strings.NewReader(value))
	if err != nil {
		return ""
	}
	renderer := markdownRenderer{}
	return cleanMarkdown(renderer.renderChildren(document, 0))
}

type markdownRenderer struct{}

func (renderer markdownRenderer) renderChildren(node *xhtml.Node, listDepth int) string {
	var result strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		result.WriteString(renderer.renderNode(child, listDepth))
	}
	return result.String()
}

func (renderer markdownRenderer) renderNode(node *xhtml.Node, listDepth int) string {
	switch node.Type {
	case xhtml.TextNode:
		return normalizeHTMLText(node.Data)
	case xhtml.DocumentNode:
		return renderer.renderChildren(node, listDepth)
	case xhtml.ElementNode:
	default:
		return ""
	}

	tag := strings.ToLower(node.Data)
	switch tag {
	case "head", "script", "style", "template", "noscript", "iframe", "object", "embed":
		return ""
	case "html", "body", "main", "section", "article", "header", "footer", "aside":
		return renderer.renderChildren(node, listDepth)
	case "p", "div", "figure":
		return markdownBlock(renderer.renderChildren(node, listDepth))
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(tag[1] - '0')
		content := strings.TrimSpace(renderer.renderChildren(node, listDepth))
		if content == "" {
			return ""
		}
		return "\n\n" + strings.Repeat("#", level) + " " + content + "\n\n"
	case "strong", "b":
		return wrapMarkdownInline("**", renderer.renderChildren(node, listDepth))
	case "em", "i":
		return wrapMarkdownInline("*", renderer.renderChildren(node, listDepth))
	case "del", "s", "strike":
		return wrapMarkdownInline("~~", renderer.renderChildren(node, listDepth))
	case "br":
		return "<br>\n"
	case "hr":
		return "\n\n---\n\n"
	case "a":
		label := strings.TrimSpace(renderer.renderChildren(node, listDepth))
		destination := safeMarkdownURL(nodeAttribute(node, "href"), false)
		if destination == "" {
			return label
		}
		if label == "" {
			label = escapeMarkdownText(destination)
		}
		return "[" + label + "](" + destination + ")"
	case "img":
		destination := safeMarkdownURL(nodeAttribute(node, "src"), true)
		alt := escapeMarkdownLabel(nodeAttribute(node, "alt"))
		if destination == "" {
			return alt
		}
		return "![" + alt + "](" + destination + ")"
	case "code":
		if node.Parent != nil && strings.EqualFold(node.Parent.Data, "pre") {
			return nodeText(node)
		}
		return inlineCode(nodeText(node))
	case "pre":
		return fencedCodeBlock(node)
	case "blockquote":
		return blockquoteMarkdown(renderer.renderChildren(node, listDepth))
	case "ul", "ol":
		return renderer.renderList(node, listDepth)
	case "li":
		return markdownBlock(renderer.renderChildren(node, listDepth))
	case "table":
		return renderer.renderTable(node)
	case "figcaption":
		return markdownBlock(wrapMarkdownInline("*", renderer.renderChildren(node, listDepth)))
	case "input", "source", "track", "meta", "link":
		return ""
	default:
		return renderer.renderChildren(node, listDepth)
	}
}

func (renderer markdownRenderer) renderList(node *xhtml.Node, depth int) string {
	ordered := strings.EqualFold(node.Data, "ol")
	number := 1
	if start, err := strconv.Atoi(nodeAttribute(node, "start")); err == nil && start > 0 {
		number = start
	}
	indent := strings.Repeat("  ", depth)
	var result strings.Builder
	for item := node.FirstChild; item != nil; item = item.NextSibling {
		if item.Type != xhtml.ElementNode || !strings.EqualFold(item.Data, "li") {
			continue
		}
		if value, err := strconv.Atoi(nodeAttribute(item, "value")); err == nil && value > 0 {
			number = value
		}
		marker := "- "
		if ordered {
			marker = strconv.Itoa(number) + ". "
		}

		var content strings.Builder
		var nestedLists []*xhtml.Node
		for child := item.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == xhtml.ElementNode &&
				(strings.EqualFold(child.Data, "ul") || strings.EqualFold(child.Data, "ol")) {
				nestedLists = append(nestedLists, child)
				continue
			}
			content.WriteString(renderer.renderNode(child, depth))
		}
		itemContent := cleanMarkdown(content.String())
		if itemContent == "" {
			itemContent = " "
		}
		lines := strings.Split(itemContent, "\n")
		result.WriteString(indent)
		result.WriteString(marker)
		result.WriteString(lines[0])
		for _, line := range lines[1:] {
			result.WriteByte('\n')
			if line != "" {
				result.WriteString(indent)
				result.WriteString(strings.Repeat(" ", len(marker)))
				result.WriteString(line)
			}
		}
		for _, nested := range nestedLists {
			result.WriteByte('\n')
			result.WriteString(strings.TrimRight(renderer.renderList(nested, depth+1), "\n"))
		}
		result.WriteByte('\n')
		if ordered {
			number++
		}
	}
	if result.Len() == 0 {
		return ""
	}
	return "\n\n" + strings.TrimRight(result.String(), "\n") + "\n\n"
}

func (renderer markdownRenderer) renderTable(node *xhtml.Node) string {
	rows := tableRows(node)
	if len(rows) == 0 {
		return ""
	}

	cellRows := make([][]string, 0, len(rows))
	headerRow := false
	columnCount := 0
	for _, row := range rows {
		cells, hasHeader := tableCells(row, renderer)
		if len(cells) == 0 {
			continue
		}
		if len(cellRows) == 0 {
			headerRow = hasHeader
		}
		if len(cells) > columnCount {
			columnCount = len(cells)
		}
		cellRows = append(cellRows, cells)
	}
	if len(cellRows) == 0 || columnCount == 0 {
		return ""
	}

	for index := range cellRows {
		for len(cellRows[index]) < columnCount {
			cellRows[index] = append(cellRows[index], "")
		}
	}
	header := make([]string, columnCount)
	body := cellRows
	if headerRow {
		header = cellRows[0]
		body = cellRows[1:]
	}
	for index, value := range header {
		if strings.TrimSpace(value) == "" {
			header[index] = " "
		}
	}

	var result strings.Builder
	result.WriteString(markdownTableRow(header))
	result.WriteByte('\n')
	separator := make([]string, columnCount)
	for index := range separator {
		separator[index] = "---"
	}
	result.WriteString(markdownTableRow(separator))
	for _, row := range body {
		result.WriteByte('\n')
		result.WriteString(markdownTableRow(row))
	}
	return "\n\n" + result.String() + "\n\n"
}

func tableRows(table *xhtml.Node) []*xhtml.Node {
	var rows []*xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != xhtml.ElementNode {
				continue
			}
			if strings.EqualFold(child.Data, "table") {
				continue
			}
			if strings.EqualFold(child.Data, "tr") {
				rows = append(rows, child)
				continue
			}
			walk(child)
		}
	}
	walk(table)
	return rows
}

func tableCells(row *xhtml.Node, renderer markdownRenderer) ([]string, bool) {
	var cells []string
	hasHeader := false
	for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
		if cell.Type != xhtml.ElementNode ||
			(!strings.EqualFold(cell.Data, "th") && !strings.EqualFold(cell.Data, "td")) {
			continue
		}
		if strings.EqualFold(cell.Data, "th") {
			hasHeader = true
		}
		value := cleanMarkdown(renderer.renderChildren(cell, 0))
		value = strings.ReplaceAll(value, "|", `\|`)
		value = strings.ReplaceAll(value, "\n", "<br>")
		cells = append(cells, strings.TrimSpace(value))
	}
	return cells, hasHeader
}

func markdownTableRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

func markdownBlock(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return "\n\n" + value + "\n\n"
}

func wrapMarkdownInline(marker string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return marker + value + marker
}

func blockquoteMarkdown(value string) string {
	value = cleanMarkdown(value)
	if value == "" {
		return ""
	}
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if line == "" {
			lines[index] = ">"
		} else {
			lines[index] = "> " + line
		}
	}
	return "\n\n" + strings.Join(lines, "\n") + "\n\n"
}

func fencedCodeBlock(node *xhtml.Node) string {
	code := strings.Trim(nodeText(node), "\n")
	if code == "" {
		return ""
	}
	language := ""
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != xhtml.ElementNode || !strings.EqualFold(child.Data, "code") {
			continue
		}
		for _, className := range strings.Fields(nodeAttribute(child, "class")) {
			if strings.HasPrefix(className, "language-") {
				language = safeCodeLanguage(strings.TrimPrefix(className, "language-"))
				break
			}
		}
		break
	}
	fence := strings.Repeat("`", maxBacktickRun(code)+1)
	if len(fence) < 3 {
		fence = "```"
	}
	return "\n\n" + fence + language + "\n" + code + "\n" + fence + "\n\n"
}

func inlineCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	marker := strings.Repeat("`", maxBacktickRun(value)+1)
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		value = " " + value + " "
	}
	return marker + value + marker
}

func maxBacktickRun(value string) int {
	maximum := 0
	current := 0
	for _, character := range value {
		if character == '`' {
			current++
			if current > maximum {
				maximum = current
			}
		} else {
			current = 0
		}
	}
	return maximum
}

func safeCodeLanguage(value string) string {
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			character != '-' && character != '_' && character != '+' {
			return ""
		}
	}
	return value
}

func nodeText(node *xhtml.Node) string {
	var result strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			result.WriteString(current.Data)
			return
		}
		if current.Type == xhtml.ElementNode && strings.EqualFold(current.Data, "br") {
			result.WriteByte('\n')
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return result.String()
}

func nodeAttribute(node *xhtml.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func safeMarkdownURL(value string, image bool) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return ""
		}
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if parsed.Scheme != "" {
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
		case "mailto", "tel":
			if image {
				return ""
			}
		default:
			return ""
		}
	}
	return strings.NewReplacer(
		`\`, "%5C",
		" ", "%20",
		"(", "%28",
		")", "%29",
		"[", "%5B",
		"]", "%5D",
		"<", "%3C",
		">", "%3E",
		"\"", "%22",
		"'", "%27",
		"`", "%60",
	).Replace(value)
}

func normalizeHTMLText(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	leadingSpace := unicode.IsSpace(runes[0])
	trailingSpace := unicode.IsSpace(runes[len(runes)-1])
	content := strings.Join(strings.Fields(value), " ")
	if content == "" {
		return " "
	}
	content = escapeMarkdownText(content)
	if leadingSpace {
		content = " " + content
	}
	if trailingSpace {
		content += " "
	}
	return content
}

func escapeMarkdownText(value string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		"`", `\`+"`",
		"*", `\*`,
		"_", `\_`,
		"[", `\[`,
		"]", `\]`,
		"<", "&lt;",
		">", "&gt;",
	).Replace(value)
}

func escapeMarkdownLabel(value string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		"[", `\[`,
		"]", `\]`,
		"<", "&lt;",
		">", "&gt;",
		"\n", " ",
		"\r", " ",
	).Replace(strings.TrimSpace(value))
}

func cleanMarkdown(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	inFence := false
	lastBlank := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
		}
		if !inFence {
			line = strings.TrimRightFunc(line, unicode.IsSpace)
		}
		if !inFence && strings.TrimSpace(line) == "" {
			if lastBlank {
				continue
			}
			result = append(result, "")
			lastBlank = true
			continue
		}
		result = append(result, line)
		lastBlank = false
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func entityNames(items []workItemEntity) []string {
	return personNames(entityPeople(items))
}

func entityPeople(items []workItemEntity) []Person {
	result := make([]Person, 0, len(items))
	for _, item := range items {
		person := entityPerson(item)
		if person.Name != "" {
			result = append(result, person)
		}
	}
	return result
}

func entityPerson(item workItemEntity) Person {
	name := fallback(
		strings.TrimSpace(item.DisplayName),
		strings.TrimSpace(item.Name),
	)
	name = fallback(name, strings.TrimSpace(item.Email))
	name = fallback(name, strings.TrimSpace(item.ID))
	return Person{
		ID:   strings.TrimSpace(item.ID),
		Name: name,
	}
}

func personNames(items []Person) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) != "" {
			result = append(result, strings.TrimSpace(item.Name))
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
