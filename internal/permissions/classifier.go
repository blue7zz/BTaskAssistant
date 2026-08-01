package permissions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxToolArgumentsBytes = 3 * 1024 * 1024

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$`)

type ToolInput struct {
	TaskID   string
	ToolName string
	Args     json.RawMessage
}

func ClassifyTool(input ToolInput) Classification {
	input.ToolName = strings.TrimSpace(input.ToolName)
	if input.TaskID == "" || !toolNamePattern.MatchString(input.ToolName) ||
		len(input.Args) == 0 || len(input.Args) > maxToolArgumentsBytes {
		return deniedClassification("工具身份或参数无法规范化")
	}
	digest, err := CanonicalArgsDigest(input.Args)
	if err != nil {
		return deniedClassification("工具参数不是有效 JSON")
	}

	switch input.ToolName {
	case "btask_list_resources":
		var args struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := decodeStrict(input.Args, &args); err != nil || len(args.Query) > 200 || args.Limit < 0 || args.Limit > 100 {
			return deniedClassification("资源列表参数无法规范化")
		}
		target, _ := NormalizeTarget(PathTarget{
			RootKind: "task-resources", RootID: input.TaskID,
			RelativePath: ".", Operation: "read",
		})
		return Classification{
			Capability: "task.resource.list", Subject: "列出当前任务资源",
			Target: "当前任务资源索引", NormalizedTarget: target, ArgsDigest: digest,
			RiskLevel: RiskLow, ReadOnly: true,
		}
	case "btask_read_resource":
		var args struct {
			ResourceID string `json:"resourceId"`
		}
		if err := decodeStrict(input.Args, &args); err != nil || !validOpaqueID(args.ResourceID) {
			return deniedClassification("资源读取目标无法规范化")
		}
		target, _ := NormalizeTarget(PathTarget{
			RootKind: "task-resources", RootID: input.TaskID,
			RelativePath: args.ResourceID, Operation: "read",
		})
		return Classification{
			Capability: "task.resource.read", Subject: "读取当前任务资源",
			Target: args.ResourceID, NormalizedTarget: target, ArgsDigest: digest,
			RiskLevel: RiskLow, ReadOnly: true,
		}
	case "btask_write_artifact":
		var args struct {
			Kind    string `json:"kind"`
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := decodeStrict(input.Args, &args); err != nil {
			return deniedClassification("artifact 参数无法规范化")
		}
		logicalPath, err := artifactPath(args.Kind, args.Name)
		if err != nil || args.Content == "" || len(args.Content) > 2*1024*1024 || !utf8.ValidString(args.Content) || strings.ContainsRune(args.Content, '\x00') {
			return deniedClassification("artifact 目标或内容不符合受控写入合同")
		}
		target, _ := NormalizeTarget(PathTarget{
			RootKind: "task-artifacts", RootID: input.TaskID,
			RelativePath: strings.TrimPrefix(logicalPath, "artifacts/"), Operation: "modify",
		})
		return Classification{
			Capability: "task.artifact.write", Subject: "写入当前任务 artifact",
			Target: logicalPath, NormalizedTarget: target, ArgsDigest: digest,
			RiskLevel: RiskMedium, Mutating: true,
		}
	case "btask_permission_probe":
		var args struct {
			Target string `json:"target"`
		}
		if err := decodeStrict(input.Args, &args); err != nil || !validOpaqueID(args.Target) {
			return deniedClassification("权限自测目标无法规范化")
		}
		target, _ := NormalizeTarget(PathTarget{
			RootKind: "permission-self-test", RootID: input.TaskID,
			RelativePath: args.Target, Operation: "read",
		})
		return Classification{
			Capability: "diagnostic.permission.probe", Subject: "验证 BTask 权限审批链路",
			Target: args.Target, NormalizedTarget: target, ArgsDigest: digest,
			RiskLevel: RiskHigh, ReadOnly: true,
		}
	default:
		return Classification{
			Capability: "unknown.tool", Subject: "调用未知工具", Target: input.ToolName,
			NormalizedTarget: "unknown:" + input.ToolName, ArgsDigest: digest,
			RiskLevel: RiskHigh, Mutating: true,
		}
	}
}

func CanonicalArgsDigest(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || len(raw) > maxToolArgumentsBytes {
		return "", errors.New("tool arguments are empty or too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("tool arguments contain trailing JSON")
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	encoded := bytes.TrimSuffix(canonical.Bytes(), []byte{'\n'})
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func NormalizeTarget(target PathTarget) (string, error) {
	for label, value := range map[string]string{
		"root kind": target.RootKind, "root id": target.RootID,
		"relative path": target.RelativePath, "operation": target.Operation,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsRune(value, '\x00') {
			return "", fmt.Errorf("permission target %s is required", label)
		}
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeStrict(raw json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func validOpaqueID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 200 && !strings.ContainsAny(value, "\x00\r\n/\\")
}

func artifactPath(kind string, name string) (string, error) {
	directory := map[string]string{
		"plan": "plans", "report": "reports", "proposal": "proposals", "export": "exports",
	}[strings.ToLower(strings.TrimSpace(kind))]
	if directory == "" {
		return "", errors.New("unsupported artifact kind")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 160 || strings.ContainsAny(name, "\x00/\\") || filepath.Base(name) != name {
		return "", errors.New("invalid artifact name")
	}
	extension := strings.ToLower(filepath.Ext(name))
	switch extension {
	case "":
		name += ".md"
	case ".md", ".txt", ".json", ".yaml", ".yml", ".csv":
	default:
		return "", errors.New("unsupported artifact extension")
	}
	return "artifacts/" + directory + "/" + name, nil
}

func deniedClassification(reason string) Classification {
	return Classification{
		Capability: "unknown.tool", Subject: "拒绝无法规范化的工具调用",
		RiskLevel: RiskCritical, Mutating: true, HardDenyReason: reason,
	}
}
