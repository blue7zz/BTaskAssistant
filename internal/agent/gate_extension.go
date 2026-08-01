package agent

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
)

const gateExtensionVersion = "btask-gate/v2"
const permissionProtocolVersion = "BTASK_PERMISSION_V1"

var gateFilenameValue = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

//go:embed btask_gate_extension.ts
var gateExtensionTemplate []byte

type gateIdentity struct {
	Version       string
	Nonce         string
	ExtensionPath string
	SHA256        string
}

type gateExtensionConfig struct {
	Version            string `json:"version"`
	PermissionProtocol string `json:"permissionProtocol"`
	Nonce              string `json:"nonce"`
	TaskID             string `json:"taskId"`
	SessionID          string `json:"sessionId"`
	Mode               string `json:"mode"`
	SelfTest           bool   `json:"selfTest,omitempty"`
}

func installGateExtension(
	workspaceRoot string,
	taskID string,
	sessionID string,
	mode string,
) (gateIdentity, error) {
	if !gateFilenameValue.MatchString(sessionID) {
		return gateIdentity{}, errors.New("PI session id cannot be used for its gate extension")
	}
	nonce := newID("gate")
	config, err := json.Marshal(gateExtensionConfig{
		Version:            gateExtensionVersion,
		PermissionProtocol: permissionProtocolVersion,
		Nonce:              nonce,
		TaskID:             taskID,
		SessionID:          sessionID,
		Mode:               mode,
	})
	if err != nil {
		return gateIdentity{}, err
	}
	marker := []byte("__BTASK_GATE_CONFIG__")
	if bytes.Count(gateExtensionTemplate, marker) != 1 {
		return gateIdentity{}, errors.New("embedded PI gate extension marker is invalid")
	}
	content := bytes.Replace(gateExtensionTemplate, marker, config, 1)
	filename := "btask-gate-v2-" + sessionID + ".ts"
	path, err := (taskspace.Service{}).InstallAgentExtension(
		filepath.Dir(workspaceRoot), taskID, filename, content,
	)
	if err != nil {
		return gateIdentity{}, fmt.Errorf("install PI gate extension: %w", err)
	}
	written, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(written, content) {
		return gateIdentity{}, errors.New("installed PI gate extension did not pass integrity verification")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return gateIdentity{}, errors.New("installed PI gate extension has an unsafe file type or mode")
	}
	if !strings.HasPrefix(path, workspaceRoot+string(filepath.Separator)) {
		return gateIdentity{}, errors.New("installed PI gate extension escaped its task workspace")
	}
	hash := sha256.Sum256(content)
	return gateIdentity{
		Version: gateExtensionVersion, Nonce: nonce, ExtensionPath: path,
		SHA256: fmt.Sprintf("%x", hash[:]),
	}, nil
}
