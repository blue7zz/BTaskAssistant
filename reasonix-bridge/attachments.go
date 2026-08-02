// attachments.go — 附件读写（阶段 6：核心矩阵清零）。
// 附件保存到任务会话目录的 attachments/ 子目录（与 JSONL 同生命周期，
// 随任务会话清理）；读取经路径归属校验（仅任务会话目录内）。

package bridge

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// attachmentDir 返回任务附件目录（<任务会话目录>/attachments）。
func (m *Manager) AttachmentDir(taskID string) string {
	return filepath.Join(m.TaskSessionDir(taskID), "attachments")
}

// SavePastedImage 保存粘贴图片（data URL）到任务附件目录，返回相对路径。
func (m *Manager) SavePastedImage(taskID string, dataURL string) (string, error) {
	data, ext, err := decodeDataURL(dataURL)
	if err != nil {
		return "", err
	}
	return m.saveAttachment(taskID, data, "image", ext)
}

// SavePastedFile 保存粘贴/拖入文件（data URL）到任务附件目录，返回相对路径。
func (m *Manager) SavePastedFile(taskID string, name string, dataURL string) (string, error) {
	data, ext, err := decodeDataURL(dataURL)
	if err != nil {
		return "", err
	}
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(name))
		if len(ext) > 8 {
			ext = ""
		}
	}
	return m.saveAttachment(taskID, data, sanitizeAttachmentName(name), ext)
}

// SaveClipboardImage 保存剪贴板图片（宿主：无剪贴板读取能力——显式错误）。
func (m *Manager) SaveClipboardImage(taskID string) (string, error) {
	return "", fmt.Errorf("Reasonix 宿主未实现: SaveClipboardImage（请使用粘贴方式插入图片）")
}

// saveAttachment 写附件文件（原子：临时文件 + rename）。
func (m *Manager) saveAttachment(taskID string, data []byte, kind string, ext string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("附件内容为空")
	}
	if len(data) > 32*1024*1024 {
		return "", fmt.Errorf("附件超过 32MB 上限")
	}
	dir := m.AttachmentDir(taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建附件目录: %w", err)
	}
	now := time.Now()
	name := fmt.Sprintf("%s-%s%s", now.Format("20060102-150405.000000000"), kind, ext)
	path := filepath.Join(dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	// 返回任务会话目录内的相对路径（AttachmentDataURL 用它拼回绝对路径）
	rel, err := filepath.Rel(m.TaskSessionDir(taskID), path)
	if err != nil {
		return "", err
	}
	return rel, nil
}

// AttachmentDataURL 读取附件并返回 data URL（仅允许任务会话目录内路径）。
func (m *Manager) AttachmentDataURL(taskID string, relPath string) (string, error) {
	sessionDir := m.TaskSessionDir(taskID)
	cleaned := filepath.Clean(relPath)
	full := cleaned
	if !filepath.IsAbs(full) {
		full = filepath.Join(sessionDir, cleaned)
	}
	// 归属校验：必须在任务会话目录内（含 attachments 子目录）
	rel, err := filepath.Rel(filepath.Clean(sessionDir), filepath.Clean(full))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("拒绝会话目录外的附件: %s", relPath)
	}
	if real, resolveErr := filepath.EvalSymlinks(filepath.Clean(full)); resolveErr == nil {
		realDir := filepath.Dir(real)
		if sessionReal, rErr := filepath.EvalSymlinks(sessionDir); rErr == nil {
			rel2, relErr2 := filepath.Rel(sessionReal, realDir)
			if relErr2 != nil || rel2 == ".." || strings.HasPrefix(rel2, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("拒绝符号链接逃逸的附件: %s", relPath)
			}
		}
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(full))
	mime := map[string]string{
		".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml",
		".pdf": "application/pdf", ".txt": "text/plain", ".md": "text/markdown",
	}[ext]
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// decodeDataURL 解析 data URL（data:<mime>;base64,<data>）。
func decodeDataURL(dataURL string) ([]byte, string, error) {
	if !strings.HasPrefix(dataURL, "data:") {
		return nil, "", fmt.Errorf("不是 data URL")
	}
	comma := strings.Index(dataURL, ",")
	if comma < 0 {
		return nil, "", fmt.Errorf("data URL 缺少数据")
	}
	header := dataURL[5:comma]
	payload := dataURL[comma+1:]
	if !strings.HasSuffix(header, ";base64") {
		return nil, "", fmt.Errorf("仅支持 base64 data URL")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("base64 解码失败: %w", err)
	}
	// 提取扩展名（mime → ext）
	ext := ""
	lower := strings.ToLower(header)
	switch {
	case strings.Contains(lower, "png"):
		ext = ".png"
	case strings.Contains(lower, "jpeg") || strings.Contains(lower, "jpg"):
		ext = ".jpg"
	case strings.Contains(lower, "gif"):
		ext = ".gif"
	case strings.Contains(lower, "webp"):
		ext = ".webp"
	case strings.Contains(lower, "svg"):
		ext = ".svg"
	case strings.Contains(lower, "pdf"):
		ext = ".pdf"
	case strings.Contains(lower, "markdown") || strings.Contains(lower, "md"):
		ext = ".md"
	case strings.Contains(lower, "text/plain"):
		ext = ".txt"
	}
	return data, ext, nil
}

// sanitizeAttachmentName 清理附件名（仅保留安全字符）。
func sanitizeAttachmentName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "_")
	if name == "" {
		name = "file"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}
