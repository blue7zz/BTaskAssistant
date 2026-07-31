package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blue7zz/BTaskAssistant/internal/credentials"
	"github.com/blue7zz/BTaskAssistant/internal/engine"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type memoryCredentialStore struct {
	secrets map[string]string
}

func TestNewAppConfiguresDirectoryDialog(t *testing.T) {
	app := NewApp()
	if app.openDirectoryDialog == nil {
		t.Fatal("expected the Wails directory dialog to be configured")
	}
	if app.emitDailyReportProgress == nil {
		t.Fatal("expected the Wails daily report progress emitter to be configured")
	}
}

func TestSelectDailyReportProjectDirectoryReturnsAbsoluteCleanPath(
	t *testing.T,
) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "desktop")
	selected := filepath.Join(".", "workspace", "..", "repository")
	app := &App{
		ctx: ctx,
		openDirectoryDialog: func(
			gotContext context.Context,
			options wailsruntime.OpenDialogOptions,
		) (string, error) {
			if gotContext != ctx {
				t.Fatal("directory dialog did not receive the Wails context")
			}
			if options.Title != "选择本地 Git 仓库" {
				t.Fatalf("unexpected dialog title %q", options.Title)
			}
			return selected, nil
		},
	}

	result, err := app.SelectDailyReportProjectDirectory()
	if err != nil {
		t.Fatalf("select daily report project directory: %v", err)
	}
	expected, err := filepath.Abs(selected)
	if err != nil {
		t.Fatalf("resolve expected absolute path: %v", err)
	}
	if result != filepath.Clean(expected) {
		t.Fatalf("expected %q, got %q", filepath.Clean(expected), result)
	}
}

func TestSelectDailyReportProjectDirectoryAllowsCancellation(t *testing.T) {
	app := &App{
		ctx: context.Background(),
		openDirectoryDialog: func(
			context.Context,
			wailsruntime.OpenDialogOptions,
		) (string, error) {
			return "", nil
		},
	}

	result, err := app.SelectDailyReportProjectDirectory()
	if err != nil {
		t.Fatalf("cancel directory selection: %v", err)
	}
	if result != "" {
		t.Fatalf("expected an empty cancelled selection, got %q", result)
	}
}

func TestSelectDailyReportProjectDirectoryRequiresDesktopContext(t *testing.T) {
	called := false
	app := &App{
		openDirectoryDialog: func(
			context.Context,
			wailsruntime.OpenDialogOptions,
		) (string, error) {
			called = true
			return "", nil
		},
	}

	_, err := app.SelectDailyReportProjectDirectory()
	if err == nil || !strings.Contains(err.Error(), "桌面客户端尚未初始化") {
		t.Fatalf("expected an uninitialised desktop error, got %v", err)
	}
	if called {
		t.Fatal("directory dialog must not open without the Wails context")
	}
}

func TestSelectDailyReportProjectDirectoryWrapsDialogError(t *testing.T) {
	dialogErr := errors.New("dialog unavailable")
	app := &App{
		ctx: context.Background(),
		openDirectoryDialog: func(
			context.Context,
			wailsruntime.OpenDialogOptions,
		) (string, error) {
			return "", dialogErr
		},
	}

	_, err := app.SelectDailyReportProjectDirectory()
	if err == nil || !strings.Contains(err.Error(), "选择本地 Git 仓库目录失败") {
		t.Fatalf("expected a wrapped directory dialog error, got %v", err)
	}
	if !errors.Is(err, dialogErr) {
		t.Fatalf("expected the dialog error to be preserved, got %v", err)
	}
}

type recordingDailyReportGenerator struct {
	input    engine.DailyReportGenerationInput
	runtime  engine.PISettings
	result   engine.DailyReportGenerationResult
	err      error
	deadline bool
}

func (generator *recordingDailyReportGenerator) Generate(
	ctx context.Context,
	input engine.DailyReportGenerationInput,
	runtime engine.PISettings,
	reportProgress engine.DailyReportProgressReporter,
) (engine.DailyReportGenerationResult, error) {
	generator.input = input
	generator.runtime = runtime
	_, generator.deadline = ctx.Deadline()
	if reportProgress != nil {
		reportProgress(engine.DailyReportGenerationProgress{
			RequestID: input.RequestID,
			Stage:     "waiting_ai",
			Message:   "已发送生成请求",
		})
	}
	return generator.result, generator.err
}

func (s *memoryCredentialStore) Set(account string, secret string) error {
	s.secrets[account] = secret
	return nil
}

func (s *memoryCredentialStore) Get(account string) (string, error) {
	secret, ok := s.secrets[account]
	if !ok {
		return "", credentials.ErrNotFound
	}
	return secret, nil
}

func (s *memoryCredentialStore) Delete(account string) error {
	delete(s.secrets, account)
	return nil
}

func TestGenerateDailyReportDelegatesWithNormalizedRuntime(t *testing.T) {
	input := engine.DailyReportGenerationInput{
		RequestID:          "daily-report-app-test",
		ReportDate:         "2026-07-30",
		Organization:       "万象",
		Level:              "L1",
		Role:               "FE",
		Engine:             "pi",
		CustomInstructions: "突出可验证结果，语言简洁",
	}
	expected := engine.DailyReportGenerationResult{
		ReportDate: "2026-07-30",
		Results: []engine.DailyReportGeneratedResult{{
			ProjectNo:   "y15",
			ProjectName: "y15_app",
			Task:        "完成日报生成",
			Status:      "已完成",
			Progress:    "100%",
			Evidence:    []string{"commit abc1234"},
		}},
	}
	generator := &recordingDailyReportGenerator{result: expected}
	var progressEvents []engine.DailyReportGenerationProgress
	app := &App{
		ctx:                  context.Background(),
		dailyReportGenerator: generator,
		emitDailyReportProgress: func(
			_ context.Context,
			progress engine.DailyReportGenerationProgress,
		) {
			progressEvents = append(progressEvents, progress)
		},
	}

	result, err := app.GenerateDailyReport(input, engine.PISettings{})
	if err != nil {
		t.Fatalf("generate daily report: %v", err)
	}
	if result.ReportDate != expected.ReportDate || len(result.Results) != 1 {
		t.Fatalf("unexpected result %#v", result)
	}
	if generator.input.ReportDate != input.ReportDate ||
		generator.input.Engine != "pi" ||
		generator.input.CustomInstructions != input.CustomInstructions {
		t.Fatalf("unexpected delegated input %#v", generator.input)
	}
	if generator.runtime.ThinkingEffort != "xhigh" ||
		generator.runtime.TimeoutMinutes != 3 {
		t.Fatalf("runtime was not normalized: %#v", generator.runtime)
	}
	if !generator.deadline {
		t.Fatal("expected App to apply a generation deadline")
	}
	if len(progressEvents) != 1 ||
		progressEvents[0].RequestID != input.RequestID ||
		progressEvents[0].Stage != "waiting_ai" {
		t.Fatalf("unexpected progress events %#v", progressEvents)
	}
}

func TestGenerateDailyReportEmitsSafeFailureProgress(t *testing.T) {
	input := engine.DailyReportGenerationInput{
		RequestID:          "daily-report-failure-test",
		ReportDate:         "2026-07-30",
		Organization:       "万象",
		Level:              "L1",
		Role:               "FE",
		Engine:             "pi",
		ManualDescription:  "测试失败回显",
		CustomInstructions: "不要泄露这段提示词",
	}
	generator := &recordingDailyReportGenerator{err: errors.New("secret backend output")}
	var progressEvents []engine.DailyReportGenerationProgress
	app := &App{
		ctx:                  context.Background(),
		dailyReportGenerator: generator,
		emitDailyReportProgress: func(
			_ context.Context,
			progress engine.DailyReportGenerationProgress,
		) {
			progressEvents = append(progressEvents, progress)
		},
	}

	_, err := app.GenerateDailyReport(input, engine.PISettings{})
	if err == nil {
		t.Fatal("expected generation failure")
	}
	if len(progressEvents) != 2 || progressEvents[1].Stage != "failed" {
		t.Fatalf("unexpected failure progress %#v", progressEvents)
	}
	if strings.Contains(progressEvents[1].Message, "secret") ||
		strings.Contains(progressEvents[1].Message, input.CustomInstructions) {
		t.Fatalf("failure progress leaked private text %#v", progressEvents[1])
	}
}

func TestSetupPlaneConnectionDiscoversProjectsAndStoresToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/api/v1/workspaces/team/projects/" {
				t.Fatalf("unexpected request path %q", request.URL.Path)
			}
			if request.Header.Get("X-API-Key") != "plane_api_test" {
				t.Fatal("missing Plane API key")
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"results": []map[string]string{{
					"id":         "project-1",
					"name":       "Team project",
					"identifier": "TEAM",
				}},
			})
		},
	))
	defer server.Close()

	store := &memoryCredentialStore{secrets: map[string]string{}}
	app := &App{credentials: store}
	setup, err := app.SetupPlaneConnection(
		server.URL+"/team/projects/project-1/",
		"plane_api_test",
	)
	if err != nil {
		t.Fatalf("setup Plane connection: %v", err)
	}
	if setup.BaseURL != server.URL || setup.WorkspaceSlug != "team" {
		t.Fatalf("unexpected setup result %#v", setup)
	}
	if len(setup.Projects) != 1 || setup.Projects[0].ID != "project-1" {
		t.Fatalf("unexpected projects %#v", setup.Projects)
	}
	account, err := planeCredentialAccount(server.URL)
	if err != nil {
		t.Fatalf("credential account: %v", err)
	}
	if store.secrets[account] != "plane_api_test" {
		t.Fatal("token was not stored under the instance account")
	}
}

func TestHasPlaneTokenMigratesLegacyCredential(t *testing.T) {
	store := &memoryCredentialStore{secrets: map[string]string{}}
	app := &App{credentials: store}
	legacyAccount, err := legacyPlaneCredentialAccount(
		"https://plane.example.com",
		"team",
	)
	if err != nil {
		t.Fatalf("legacy credential account: %v", err)
	}
	store.secrets[legacyAccount] = "plane_api_legacy"

	found, err := app.HasPlaneToken("https://plane.example.com", "team")
	if err != nil {
		t.Fatalf("has Plane token: %v", err)
	}
	if !found {
		t.Fatal("expected the legacy token to be found")
	}
	account, err := planeCredentialAccount("https://plane.example.com")
	if err != nil {
		t.Fatalf("credential account: %v", err)
	}
	if store.secrets[account] != "plane_api_legacy" {
		t.Fatal("legacy token was not copied to the instance account")
	}
}

func TestDailyReportTokenUsesNormalizedURLAndEmployeeID(t *testing.T) {
	store := &memoryCredentialStore{secrets: map[string]string{}}
	app := &App{credentials: store}
	if err := app.SaveDailyReportToken(
		" https://REPORT.example.com:443/api/v1/./report/submit/ ",
		" DN1111 ",
		" daily_token_test ",
	); err != nil {
		t.Fatalf("save daily report token: %v", err)
	}
	if len(store.secrets) != 1 {
		t.Fatalf("expected one stored credential, got %d", len(store.secrets))
	}
	account, err := dailyReportCredentialAccount(
		"https://report.example.com/api/v1/report/submit",
		"DN1111",
	)
	if err != nil {
		t.Fatalf("daily report credential account: %v", err)
	}
	if store.secrets[account] != "daily_token_test" {
		t.Fatal("token was not stored under the normalized URL and employee account")
	}

	found, err := app.HasDailyReportToken(
		"https://report.example.com/api/v1/report/submit",
		"DN1111",
	)
	if err != nil {
		t.Fatalf("has daily report token: %v", err)
	}
	if !found {
		t.Fatal("expected saved daily report token to be found")
	}
	found, err = app.HasDailyReportToken(
		"https://report.example.com/api/v1/report/submit",
		"DN2222",
	)
	if err != nil {
		t.Fatalf("has other employee daily report token: %v", err)
	}
	if found {
		t.Fatal("token must not be shared with another employee ID")
	}

	if err := app.DeleteDailyReportToken(
		"https://report.example.com/api/v1/report/submit/",
		" DN1111 ",
	); err != nil {
		t.Fatalf("delete daily report token: %v", err)
	}
	found, err = app.HasDailyReportToken(
		"https://report.example.com/api/v1/report/submit",
		"DN1111",
	)
	if err != nil {
		t.Fatalf("has deleted daily report token: %v", err)
	}
	if found {
		t.Fatal("expected deleted daily report token to be absent")
	}
}

func TestSubmitDailyReportReadsEmployeeScopedCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.Header.Get("Authorization") != "Bearer daily_token_test" {
			t.Fatal("missing employee-scoped daily report token")
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode daily report request: %v", err)
		}
		if payload["emp_id"] != "DN1111" ||
			payload["report_date"] != "2026-07-30" ||
			payload["report_type"] != "日报" ||
			payload["content"] != "# 日报\n\n已完成后端接入" ||
			payload["token"] != "daily_token_test" {
			t.Fatalf("unexpected daily report payload %#v", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"code": 0,
			"msg":  "提交成功",
			"data": map[string]any{
				"id":     7,
				"action": "inserted",
			},
		})
	}))
	defer server.Close()

	store := &memoryCredentialStore{secrets: map[string]string{}}
	app := &App{credentials: store}
	if err := app.SaveDailyReportToken(
		server.URL,
		"DN1111",
		"daily_token_test",
	); err != nil {
		t.Fatalf("save daily report token: %v", err)
	}
	result, err := app.SubmitDailyReport(
		server.URL+"/",
		" DN1111 ",
		"2026-07-30",
		"# 日报\n\n已完成后端接入",
	)
	if err != nil {
		t.Fatalf("submit daily report: %v", err)
	}
	if result.ID != float64(7) || result.Action != "inserted" ||
		result.Message != "提交成功" {
		t.Fatalf("unexpected daily report result %#v", result)
	}
}

func TestSubmitDailyReportReturnsAPIMessageAsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"code": 401,
			"msg":  "token 与工号不匹配",
		})
	}))
	defer server.Close()

	store := &memoryCredentialStore{secrets: map[string]string{}}
	app := &App{credentials: store}
	if err := app.SaveDailyReportToken(
		server.URL,
		"DN1111",
		"daily_token_test",
	); err != nil {
		t.Fatalf("save daily report token: %v", err)
	}
	_, err := app.SubmitDailyReport(
		server.URL,
		"DN1111",
		"2026-07-30",
		"日报正文",
	)
	if err == nil || !strings.Contains(err.Error(), "code 401") ||
		!strings.Contains(err.Error(), "token 与工号不匹配") {
		t.Fatalf("expected API code and message, got %v", err)
	}
}

func TestSubmitDailyReportDoesNotReuseAnotherEmployeeToken(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		requests.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &memoryCredentialStore{secrets: map[string]string{}}
	app := &App{credentials: store}
	if err := app.SaveDailyReportToken(
		server.URL,
		"DN1111",
		"daily_token_test",
	); err != nil {
		t.Fatalf("save daily report token: %v", err)
	}
	_, err := app.SubmitDailyReport(
		server.URL,
		"DN2222",
		"2026-07-30",
		"日报正文",
	)
	if err == nil || !strings.Contains(err.Error(), "尚未保存") {
		t.Fatalf("expected missing employee token error, got %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("unexpected report requests: %d", requests.Load())
	}
}
