package report

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

type DailyReport struct {
	EmployeeID string
	ReportDate string
	Content    string
}

type SubmitData struct {
	ID     any    `json:"id"`
	Action string `json:"action"`
}

type SubmitResponse struct {
	Code int        `json:"code"`
	Msg  string     `json:"msg"`
	Data SubmitData `json:"data"`
}

type SubmitResult struct {
	ID      any    `json:"id"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

type Client struct {
	apiURL     *url.URL
	token      string
	httpClient *http.Client
}

func NormalizeAPIURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("请输入日报 API 地址")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("日报 API 地址格式不正确")
	}
	if parsed.User != nil {
		return "", errors.New("日报 API 地址不能包含用户名或密码")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", errors.New("日报 API 地址不能包含查询参数")
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	switch parsed.Scheme {
	case "https":
	case "http":
		host := strings.ToLower(parsed.Hostname())
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", errors.New("日报 Token 只能发送到 HTTPS；HTTP 仅允许 localhost 或 loopback 地址")
		}
	default:
		return "", errors.New("日报 API 地址必须使用 HTTPS")
	}

	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") ||
		(parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		parsed.Host = "[" + host + "]"
	} else {
		parsed.Host = host
	}
	parsed.Fragment = ""
	parsed.Path = path.Clean(parsed.Path)
	if parsed.Path == "." || parsed.Path == "/" {
		parsed.Path = ""
	} else {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}
	parsed.RawPath = ""
	return parsed.String(), nil
}

func NewClient(apiURL string, token string) (*Client, error) {
	normalized, err := NormalizeAPIURL(apiURL)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("日报 Token 尚未配置")
	}
	parsed, _ := url.Parse(normalized)
	return &Client{
		apiURL: parsed,
		token:  token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("日报 API 返回了重定向；已阻止转发 Token")
			},
		},
	}, nil
}

func (c *Client) SubmitDaily(
	ctx context.Context,
	daily DailyReport,
) (SubmitResponse, error) {
	daily.EmployeeID = strings.TrimSpace(daily.EmployeeID)
	daily.ReportDate = strings.TrimSpace(daily.ReportDate)
	if daily.EmployeeID == "" {
		return SubmitResponse{}, errors.New("请输入工号")
	}
	if daily.ReportDate == "" {
		return SubmitResponse{}, errors.New("请选择日报日期")
	}
	if strings.TrimSpace(daily.Content) == "" {
		return SubmitResponse{}, errors.New("日报内容不能为空")
	}

	payload := struct {
		EmployeeID string `json:"emp_id"`
		ReportDate string `json:"report_date"`
		ReportType string `json:"report_type"`
		Content    string `json:"content"`
		Token      string `json:"token"`
	}{
		EmployeeID: daily.EmployeeID,
		ReportDate: daily.ReportDate,
		ReportType: "日报",
		Content:    daily.Content,
		Token:      c.token,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("生成日报提交内容失败: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.apiURL.String(),
		bytes.NewReader(body),
	)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("创建日报请求失败: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("提交日报失败: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("读取日报响应失败: %w", err)
	}
	if len(responseBody) > maxResponseBytes {
		return SubmitResponse{}, errors.New("日报 API 响应过大")
	}
	var result SubmitResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return SubmitResponse{}, fmt.Errorf(
			"日报 API 返回无效 JSON（HTTP %d）: %w",
			response.StatusCode,
			err,
		)
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		if result.Code != 0 {
			return result, nil
		}
		detail := strings.TrimSpace(result.Msg)
		if detail != "" {
			detail = ": " + detail
		}
		return SubmitResponse{}, fmt.Errorf(
			"日报 API 请求失败（HTTP %d）%s",
			response.StatusCode,
			detail,
		)
	}
	return result, nil
}
