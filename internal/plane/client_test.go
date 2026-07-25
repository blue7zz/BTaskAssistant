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

func TestListCandidatesPaginatesAndPreservesSource(t *testing.T) {
	var workItemRequests atomic.Int32
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
	if commentRequests.Load() != 3 {
		t.Fatalf("expected three comment pages, got %d", commentRequests.Load())
	}
	if len(candidates) != 2 {
		t.Fatalf("expected two candidates, got %d", len(candidates))
	}
	if candidates[0].ExternalKey != "APP-8" || candidates[0].Priority != "high" {
		t.Fatalf("unexpected first candidate: %#v", candidates[0])
	}
	if !strings.Contains(candidates[0].SourceMarkdown, "保留 原始 描述") {
		t.Fatalf("source did not preserve description: %q", candidates[0].SourceMarkdown)
	}
	if len(candidates[0].AssigneeDetails) != 1 ||
		candidates[0].AssigneeDetails[0].ID != "user-alice" {
		t.Fatalf("assignee details were not preserved: %#v", candidates[0].AssigneeDetails)
	}
	if len(candidates[0].Comments) != 2 ||
		candidates[0].Comments[0].Actor.Name != "Reviewer" {
		t.Fatalf("comments were not preserved: %#v", candidates[0].Comments)
	}
	if !strings.Contains(candidates[0].SourceMarkdown, "## Plane 评论") ||
		!strings.Contains(candidates[0].SourceMarkdown, "请先覆盖登录失败路径") {
		t.Fatalf("source did not include comments: %q", candidates[0].SourceMarkdown)
	}
	if !strings.Contains(candidates[1].DescriptionMarkdown, "```json") {
		t.Fatalf("structured description was not preserved: %q", candidates[1].DescriptionMarkdown)
	}
	if candidates[1].ExternalKey != "TEAM-9" {
		t.Fatalf("expected configured project identifier, got %q", candidates[1].ExternalKey)
	}
}

func TestListCandidatesKeepsWorkItemWhenCommentSyncFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(request.URL.Path, "/work-items/") {
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"next_page_results": false,
				"results": []map[string]any{{
					"id":          "item-1",
					"name":        "保留候选",
					"sequence_id": 1,
				}},
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
	candidates, err := client.ListCandidates(
		context.Background(),
		"team",
		"project",
		"TEAM",
	)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected the work item to remain available, got %#v", candidates)
	}
	if candidates[0].CommentsSyncError == "" {
		t.Fatal("expected a visible comment sync error")
	}
	if !strings.Contains(candidates[0].SourceMarkdown, "评论同步失败") {
		t.Fatalf("source did not preserve the warning: %q", candidates[0].SourceMarkdown)
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
