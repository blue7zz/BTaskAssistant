package permissions

import (
	"net"
	"net/url"
	"sort"
	"strings"
)

type NetworkInput struct {
	Method         string
	URL            string
	HasCredentials bool
}

func ClassifyNetwork(input NetworkInput) Classification {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	rawURL := strings.TrimSpace(input.URL)
	if method == "" || rawURL == "" || strings.ContainsRune(rawURL, '\x00') {
		return deniedClassification("网络方法或目标无法规范化")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return deniedClassification("网络目标不是无凭据的绝对 URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return deniedClassification("网络目标只允许 HTTP 或 HTTPS")
	}
	if parsed.Scheme == "http" && input.HasCredentials && !loopbackHost(parsed.Hostname()) {
		return deniedClassification("带凭据的远端请求必须使用 HTTPS")
	}
	if containsCredentialQuery(parsed.Query()) {
		return deniedClassification("网络目标不得在查询参数中携带凭据")
	}

	queryNames := make([]string, 0, len(parsed.Query()))
	for key := range parsed.Query() {
		queryNames = append(queryNames, key)
	}
	sort.Strings(queryNames)
	query := make(url.Values, len(queryNames))
	for _, key := range queryNames {
		query.Set(key, "")
	}
	parsed.RawQuery = query.Encode()
	parsed.ForceQuery = false
	normalized := method + " " + parsed.String()
	digest, _ := CanonicalArgsDigest([]byte(
		`{"hasCredentials":` + boolText(input.HasCredentials) +
			`,"method":` + quoteJSON(method) + `,"url":` + quoteJSON(rawURL) + `}`,
	))
	classification := Classification{
		Capability: "network.access", Subject: "访问网络资源",
		Target:           parsed.String(),
		NormalizedTarget: normalized, ArgsDigest: digest,
		RiskLevel: RiskHigh,
	}
	switch method {
	case "GET", "HEAD", "OPTIONS":
		classification.ReadOnly = true
	case "DELETE":
		classification.Capability = "network.remote.delete"
		classification.Subject = "删除远端资源"
		classification.RiskLevel = RiskCritical
		classification.Mutating = true
	case "POST", "PUT", "PATCH":
		classification.Capability = "network.external.write"
		classification.Subject = "写入远端资源"
		classification.Mutating = true
	default:
		return deniedClassification("网络方法不在允许分类中")
	}
	return classification
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func containsCredentialQuery(query url.Values) bool {
	for key := range query {
		if credentialKey(key) {
			return true
		}
	}
	return false
}

func credentialKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return key == "pat" || key == "sig" || strings.Contains(key, "token") ||
		strings.Contains(key, "password") || strings.Contains(key, "secret") ||
		strings.Contains(key, "api_key") || strings.Contains(key, "api-key") ||
		strings.Contains(key, "apikey") || strings.Contains(key, "authorization") ||
		strings.Contains(key, "cookie") || strings.Contains(key, "credential") ||
		strings.Contains(key, "private_key") || strings.Contains(key, "private-key") ||
		strings.Contains(key, "signature")
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
