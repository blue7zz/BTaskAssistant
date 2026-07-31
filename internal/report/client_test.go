package report

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNormalizeAPIURLAllowsHTTPSAndLoopbackHTTP(t *testing.T) {
	value, err := NormalizeAPIURL("  https://REPORT.example.com:443/api/./submit/#section  ")
	if err != nil {
		t.Fatalf("normalize HTTPS API URL: %v", err)
	}
	if value != "https://report.example.com/api/submit" {
		t.Fatalf("unexpected normalized URL %q", value)
	}

	for _, raw := range []string{
		"http://localhost:8080/report",
		"http://127.0.0.1:8080/report",
		"http://[::1]:8080/report",
	} {
		if _, err := NormalizeAPIURL(raw); err != nil {
			t.Fatalf("expected loopback URL %q to be allowed: %v", raw, err)
		}
	}
}

func TestNormalizeAPIURLRejectsInsecureRemoteHTTP(t *testing.T) {
	if _, err := NormalizeAPIURL("http://reports.example.com/submit"); err == nil {
		t.Fatal("expected remote HTTP API URL to be rejected")
	}
	if _, err := NormalizeAPIURL(
		"https://reports.example.com/submit?token=secret",
	); err == nil {
		t.Fatal("expected API URL query parameters to be rejected")
	}
}

func TestSubmitDailyPostsReferenceContractAndParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %q", request.Method)
		}
		if request.URL.Path != "/api/v1/report/submit" {
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer daily_token_test" {
			t.Fatal("missing Bearer token")
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content type %q", request.Header.Get("Content-Type"))
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["emp_id"] != "DN1111" ||
			payload["report_date"] != "2026-07-30" ||
			payload["report_type"] != "日报" ||
			payload["content"] != "# 日报\n\n正文" ||
			payload["token"] != "daily_token_test" {
			t.Fatalf("unexpected request payload %#v", payload)
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"code": 0,
			"msg":  "提交成功",
			"data": map[string]any{
				"id":     42,
				"action": "updated",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(
		server.URL+"/api/v1/report/submit/",
		" daily_token_test ",
	)
	if err != nil {
		t.Fatalf("new report client: %v", err)
	}
	result, err := client.SubmitDaily(context.Background(), DailyReport{
		EmployeeID: " DN1111 ",
		ReportDate: " 2026-07-30 ",
		Content:    "# 日报\n\n正文",
	})
	if err != nil {
		t.Fatalf("submit daily report: %v", err)
	}
	if result.Code != 0 || result.Msg != "提交成功" ||
		result.Data.ID != float64(42) || result.Data.Action != "updated" {
		t.Fatalf("unexpected submit response %#v", result)
	}
}

func TestSubmitDailyReturnsParsedAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"code": 422,
			"msg":  "只能补提交最近 3 天的报告",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "daily_token_test")
	if err != nil {
		t.Fatalf("new report client: %v", err)
	}
	result, err := client.SubmitDaily(context.Background(), DailyReport{
		EmployeeID: "DN1111",
		ReportDate: "2026-07-20",
		Content:    "过期日报",
	})
	if err != nil {
		t.Fatalf("expected a parsed API response, got %v", err)
	}
	if result.Code != 422 || !strings.Contains(result.Msg, "最近 3 天") {
		t.Fatalf("unexpected API failure %#v", result)
	}
}

func TestSubmitDailyRejectsHTTPFailureWithoutBusinessCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"msg": "upstream unavailable",
			"data": map[string]any{
				"id":     99,
				"action": "inserted",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "daily_token_test")
	if err != nil {
		t.Fatalf("new report client: %v", err)
	}
	_, err = client.SubmitDaily(context.Background(), DailyReport{
		EmployeeID: "DN1111",
		ReportDate: "2026-07-30",
		Content:    "日报正文",
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") ||
		!strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("expected HTTP failure, got %v", err)
	}
}

func TestSubmitDailyRefusesRedirectBeforeForwardingToken(t *testing.T) {
	var redirectedRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		redirectedRequests.Add(1)
		if request.Header.Get("Authorization") != "" {
			t.Fatal("token was forwarded to redirect target")
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "daily_token_test")
	if err != nil {
		t.Fatalf("new report client: %v", err)
	}
	_, err = client.SubmitDaily(context.Background(), DailyReport{
		EmployeeID: "DN1111",
		ReportDate: "2026-07-30",
		Content:    "日报正文",
	})
	if err == nil || !strings.Contains(err.Error(), "重定向") {
		t.Fatalf("expected redirect refusal, got %v", err)
	}
	if redirectedRequests.Load() != 0 {
		t.Fatalf("redirect target received %d requests", redirectedRequests.Load())
	}
}
