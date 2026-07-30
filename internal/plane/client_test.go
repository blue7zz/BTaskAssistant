package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestListCandidatesLoadsSummariesAndDefersDetails(t *testing.T) {
	var workItemRequests atomic.Int32
	var detailRequests atomic.Int32
	var commentRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-API-Key") != "plane_api_test" {
			t.Fatalf("missing API key header")
		}
		writer.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(
			request.URL.Path,
			"/api/v1/workspaces/team/projects/project/work-items/",
		) {
			workItemRequests.Add(1)
			if strings.Contains(request.URL.Query().Get("fields"), "description") {
				t.Fatalf("summary request should not include description fields")
			}
			cursor := request.URL.Query().Get("cursor")
			if cursor == "" {
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"next_cursor":       "page-2",
					"next_page_results": true,
					"total_results":     2,
					"results": []map[string]any{{
						"id":                 "item-1",
						"name":               "修复登录",
						"description":        "<p>保留 <strong>原始</strong> 描述</p>",
						"priority":           "high",
						"sequence_id":        8,
						"project_identifier": "APP",
						"state":              map[string]any{"name": "进行中"},
						"labels":             []map[string]any{{"name": "bug"}},
						"assignees": []map[string]any{{
							"id":           "user-alice",
							"display_name": "Alice",
						}},
					}},
				})
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"next_page_results": false,
				"results": []map[string]any{{
					"id":          "item-2",
					"name":        "补充文档",
					"description": map[string]any{"type": "doc"},
					"priority":    "low",
					"sequence_id": 9,
					"state":       map[string]any{"name": "待办"},
				}},
			})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/work-items/item-1/") {
			detailRequests.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"id":                 "item-1",
				"name":               "修复登录",
				"description":        "<p>保留 <strong>原始</strong> 描述</p>",
				"priority":           "high",
				"sequence_id":        8,
				"project_identifier": "APP",
				"state": map[string]any{
					"name":  "进行中",
					"group": "started",
				},
				"labels": []map[string]any{{"name": "bug"}},
				"assignees": []map[string]any{{
					"id":           "user-alice",
					"display_name": "Alice",
				}},
			})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/work-items/item-1/comments/") {
			commentRequests.Add(1)
			if request.URL.Query().Get("cursor") == "" {
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"next_cursor":       "comment-page-2",
					"next_page_results": true,
					"results": []map[string]any{{
						"id":               "comment-1",
						"comment_html":     "<p>请先覆盖登录失败路径。</p>",
						"comment_stripped": "请先覆盖登录失败路径。",
						"actor_detail": map[string]any{
							"id":           "user-reviewer",
							"display_name": "Reviewer",
						},
						"created_at": "2026-07-24T10:00:00Z",
					}},
				})
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"next_page_results": false,
				"results": []map[string]any{{
					"id":           "comment-2",
					"comment_html": "<p>修复后我再复查。</p>",
					"actor": map[string]any{
						"id":           "user-alice",
						"display_name": "Alice",
					},
					"created_at": "2026-07-24T11:00:00Z",
					"edited_at":  "2026-07-24T11:05:00Z",
				}},
			})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/work-items/item-2/comments/") {
			commentRequests.Add(1)
			_ = json.NewEncoder(writer).Encode([]map[string]any{})
			return
		}
		t.Fatalf("unexpected request path %q", request.URL.Path)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "plane_api_test")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	candidates, err := client.ListCandidates(
		context.Background(),
		"team",
		"project",
		"TEAM",
	)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if workItemRequests.Load() != 2 {
		t.Fatalf("expected two work item pages, got %d", workItemRequests.Load())
	}
	if commentRequests.Load() != 0 {
		t.Fatalf("summary collection should not request comments, got %d", commentRequests.Load())
	}
	if len(candidates) != 2 {
		t.Fatalf("expected two candidates, got %d", len(candidates))
	}
	if candidates[0].ExternalKey != "APP-8" || candidates[0].Priority != "high" {
		t.Fatalf("unexpected first candidate: %#v", candidates[0])
	}
	if candidates[0].DetailsLoaded {
		t.Fatal("summary candidate should not be marked as detailed")
	}
	if len(candidates[0].AssigneeDetails) != 1 ||
		candidates[0].AssigneeDetails[0].ID != "user-alice" {
		t.Fatalf("assignee details were not preserved: %#v", candidates[0].AssigneeDetails)
	}
	if len(candidates[0].Comments) != 0 {
		t.Fatalf("summary candidate should not contain comments: %#v", candidates[0].Comments)
	}
	if strings.Contains(candidates[0].SourceMarkdown, "Plane 原始描述") ||
		strings.Contains(candidates[0].SourceMarkdown, "Plane 评论") {
		t.Fatalf("summary source unexpectedly contains details: %q", candidates[0].SourceMarkdown)
	}
	if candidates[1].ExternalKey != "TEAM-9" {
		t.Fatalf("expected configured project identifier, got %q", candidates[1].ExternalKey)
	}

	details, err := client.LoadCandidateDetails(
		context.Background(),
		"team",
		"project",
		"TEAM",
		"item-1",
	)
	if err != nil {
		t.Fatalf("load candidate details: %v", err)
	}
	if detailRequests.Load() != 1 {
		t.Fatalf("expected one detail request, got %d", detailRequests.Load())
	}
	if commentRequests.Load() != 2 {
		t.Fatalf("expected two comment pages for the selected item, got %d", commentRequests.Load())
	}
	if !details.DetailsLoaded || details.StateGroup != "started" {
		t.Fatalf("details were not marked or normalized: %#v", details)
	}
	if !strings.Contains(details.DescriptionMarkdown, "保留 **原始** 描述") {
		t.Fatalf("details did not preserve description: %q", details.DescriptionMarkdown)
	}
	if len(details.Comments) != 2 || details.Comments[0].Actor.Name != "Reviewer" {
		t.Fatalf("comments were not preserved: %#v", details.Comments)
	}
	if !strings.Contains(details.SourceMarkdown, "## Plane 评论") ||
		!strings.Contains(details.SourceMarkdown, "请先覆盖登录失败路径") {
		t.Fatalf("detailed source did not include comments: %q", details.SourceMarkdown)
	}
}

func TestLoadCandidateDetailsKeepsWorkItemWhenCommentSyncFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(request.URL.Path, "/work-items/item-1/") {
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"id":          "item-1",
				"name":        "保留候选",
				"description": "即使评论失败也保留详情。",
				"sequence_id": 1,
			})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/work-items/item-1/comments/") {
			http.Error(writer, `{"detail":"comments scope missing"}`, http.StatusForbidden)
			return
		}
		t.Fatalf("unexpected request path %q", request.URL.Path)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "plane_api_test")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	details, err := client.LoadCandidateDetails(
		context.Background(),
		"team",
		"project",
		"TEAM",
		"item-1",
	)
	if err != nil {
		t.Fatalf("load candidate details: %v", err)
	}
	if !details.DetailsLoaded || details.ExternalID != "item-1" {
		t.Fatalf("expected the work item details to remain available, got %#v", details)
	}
	if details.CommentsSyncError == "" {
		t.Fatal("expected a visible comment sync error")
	}
	if !strings.Contains(details.SourceMarkdown, "评论同步失败") {
		t.Fatalf("source did not preserve the warning: %q", details.SourceMarkdown)
	}
}

func TestLoadCandidateDetailsRejectsMismatchedWorkItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":          "different-item",
			"name":        "错误的工作项",
			"sequence_id": 2,
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "plane_api_test")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.LoadCandidateDetails(
		context.Background(),
		"team",
		"project",
		"TEAM",
		"item-1",
	)
	if err == nil || !strings.Contains(err.Error(), "ID 不匹配") {
		t.Fatalf("expected mismatched work item ID error, got %v", err)
	}
}

func TestLoadCandidateDetailsLoadsParentReference(t *testing.T) {
	tests := []struct {
		name        string
		parentField string
		parentValue any
	}{
		{name: "parent ID", parentField: "parent", parentValue: "item-parent"},
		{name: "parent_id", parentField: "parent_id", parentValue: "item-parent"},
		{
			name:        "expanded parent",
			parentField: "parent",
			parentValue: map[string]any{"id": "item-parent"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var parentRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				writer.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(request.URL.Path, "/work-items/item-child/"):
					payload := map[string]any{
						"id":          "item-child",
						"name":        "子任务",
						"description": "子任务详情",
						"sequence_id": 42,
					}
					payload[test.parentField] = test.parentValue
					_ = json.NewEncoder(writer).Encode(payload)
				case strings.HasSuffix(request.URL.Path, "/work-items/item-parent/"):
					parentRequests.Add(1)
					_ = json.NewEncoder(writer).Encode(map[string]any{
						"id":                 "item-parent",
						"name":               "父任务标题",
						"sequence_id":        8,
						"project_identifier": "PARENT",
					})
				case strings.HasSuffix(request.URL.Path, "/work-items/item-child/comments/"):
					_ = json.NewEncoder(writer).Encode([]map[string]any{})
				default:
					t.Fatalf("unexpected request path %q", request.URL.Path)
				}
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "plane_api_test")
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			candidate, err := client.LoadCandidateDetails(
				context.Background(),
				"team",
				"project",
				"TEAM",
				"item-child",
			)
			if err != nil {
				t.Fatalf("load candidate details: %v", err)
			}
			if parentRequests.Load() != 1 {
				t.Fatalf("expected one parent request, got %d", parentRequests.Load())
			}
			if candidate.Parent == nil {
				t.Fatal("expected parent reference")
			}
			if candidate.Parent.ExternalID != "item-parent" ||
				candidate.Parent.ExternalKey != "PARENT-8" ||
				candidate.Parent.Title != "父任务标题" {
				t.Fatalf("unexpected parent reference %#v", candidate.Parent)
			}
		})
	}
}

func TestLoadCandidateDetailsKeepsChildWhenParentIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(request.URL.Path, "/work-items/item-child/"):
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"id":          "item-child",
				"name":        "仍可预览的子任务",
				"description": "父任务不可用时仍保留正文。",
				"sequence_id": 42,
				"parent":      "item-missing-parent",
			})
		case strings.HasSuffix(request.URL.Path, "/work-items/item-missing-parent/"):
			http.Error(writer, `{"detail":"not found"}`, http.StatusNotFound)
		case strings.HasSuffix(request.URL.Path, "/work-items/item-child/comments/"):
			_ = json.NewEncoder(writer).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "plane_api_test")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	candidate, err := client.LoadCandidateDetails(
		context.Background(),
		"team",
		"project",
		"TEAM",
		"item-child",
	)
	if err != nil {
		t.Fatalf("parent failure should not block child details: %v", err)
	}
	if candidate.Parent != nil {
		t.Fatalf("unavailable parent should be omitted, got %#v", candidate.Parent)
	}
	if !candidate.DetailsLoaded ||
		!strings.Contains(candidate.DescriptionMarkdown, "仍保留正文") {
		t.Fatalf("child details were not preserved: %#v", candidate)
	}
}

func TestDescriptionHTMLPreservesRichMarkdownAndDropsUnsafeContent(t *testing.T) {
	item := workItem{
		ID:          "item-rich",
		Name:        "富文本任务",
		SequenceID:  9,
		Description: json.RawMessage(`{"type":"doc"}`),
		DescriptionHTML: `<h2>功能说明</h2>
<p>支持 <strong>粗体</strong>、<em>斜体</em>、<a href="https://example.com/docs?a=1&amp;b=2">链接</a> 和 <code>inline()</code>。</p>
<ul><li>第一项</li><li>第二项</li></ul>
<ol start="3"><li>第三步</li></ol>
<p><img src="https://example.com/diagram (1).png" alt="流程图"></p>
<pre><code class="language-go">fmt.Println("ok")
</code></pre>
<blockquote><p>请保留引用</p></blockquote>
<table><thead><tr><th>字段</th><th>类型</th></tr></thead><tbody><tr><td>name</td><td>string</td></tr></tbody></table>
<script>alert("script")</script>
<p>&lt;img src=x onerror=alert(1)&gt;</p>
<p><a href="javascript:alert(1)">不安全链接</a><img src="data:image/png;base64,unsafe" alt="危险图片"></p>`,
	}

	candidate := normalizeWorkItem(item, "TEAM", true)
	markdown := candidate.DescriptionMarkdown
	for _, expected := range []string{
		"## 功能说明",
		"**粗体**",
		"*斜体*",
		"[链接](https://example.com/docs?a=1&b=2)",
		"`inline()`",
		"- 第一项",
		"- 第二项",
		"3. 第三步",
		"![流程图](https://example.com/diagram%20%281%29.png)",
		"```go\nfmt.Println(\"ok\")\n```",
		"> 请保留引用",
		"| 字段 | 类型 |",
		"| --- | --- |",
		"| name | string |",
		"不安全链接",
	} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("expected Markdown to contain %q, got:\n%s", expected, markdown)
		}
	}
	for _, unsafe := range []string{"javascript:", "data:image", `alert("script")`} {
		if strings.Contains(markdown, unsafe) {
			t.Fatalf("Markdown retained unsafe content %q:\n%s", unsafe, markdown)
		}
	}
	if strings.Contains(markdown, `"type": "doc"`) {
		t.Fatalf("description_html should take precedence over raw JSON: %s", markdown)
	}
	if strings.Contains(markdown, "<img src=x") ||
		!strings.Contains(markdown, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("encoded HTML text was not kept inert: %s", markdown)
	}
	if !strings.Contains(candidate.SourceMarkdown, markdown) {
		t.Fatal("source Markdown did not retain the converted rich description")
	}
}

func TestListProjectsNormalizesAndDeduplicates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-API-Key") != "plane_api_test" {
			t.Fatalf("missing API key header")
		}
		if !strings.HasSuffix(request.URL.Path, "/api/v1/workspaces/team/projects/") {
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "project-2", "name": " Zebra ", "identifier": "ZEBRA"},
				{"id": "project-1", "name": "myriad", "identifier": "MYRIA"},
				{"id": "project-1", "name": "duplicate", "identifier": "DUP"},
				{"id": "", "name": "missing ID", "identifier": "NONE"},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "plane_api_test")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	projects, err := client.ListProjects(context.Background(), "team")
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected two unique projects, got %#v", projects)
	}
	if projects[0].ID != "project-1" || projects[0].Identifier != "MYRIA" {
		t.Fatalf("expected projects sorted by name, got %#v", projects)
	}
}

func TestNormalizeBaseURLRejectsInsecureRemoteHTTP(t *testing.T) {
	if _, err := NormalizeBaseURL("http://plane.example.com"); err == nil {
		t.Fatal("expected insecure remote HTTP to be rejected")
	}
	if value, err := NormalizeBaseURL("http://127.0.0.1:8080/"); err != nil || value == "" {
		t.Fatalf("expected loopback HTTP to be allowed, got %q and %v", value, err)
	}
}

func TestResolveWorkspaceURLExtractsSlugFromPlanePage(t *testing.T) {
	setup, err := ResolveWorkspaceURL(
		"https://plane.example.com/my-team/projects/project-id/issues/",
	)
	if err != nil {
		t.Fatalf("resolve workspace URL: %v", err)
	}
	if setup.BaseURL != "https://plane.example.com" {
		t.Fatalf("unexpected base URL %q", setup.BaseURL)
	}
	if setup.WorkspaceSlug != "my-team" {
		t.Fatalf("unexpected workspace slug %q", setup.WorkspaceSlug)
	}
}

func TestResolveWorkspaceURLRequiresWorkspacePage(t *testing.T) {
	if _, err := ResolveWorkspaceURL("https://plane.example.com/"); err == nil {
		t.Fatal("expected root URL without workspace slug to be rejected")
	}
	if _, err := ResolveWorkspaceURL(
		"https://plane.example.com/api/v1/",
	); err == nil {
		t.Fatal("expected API URL without workspace slug to be rejected")
	}
}

func TestResolveWorkspaceURLMapsPlaneCloudWebHostToAPIHost(t *testing.T) {
	setup, err := ResolveWorkspaceURL("https://app.plane.so/my-team/")
	if err != nil {
		t.Fatalf("resolve Plane Cloud URL: %v", err)
	}
	if setup.BaseURL != "https://api.plane.so" {
		t.Fatalf("unexpected Plane Cloud API URL %q", setup.BaseURL)
	}
}
