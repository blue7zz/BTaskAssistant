package taskspace

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxPreviewBytes = 2 * 1024 * 1024

func ValidateLogicalPath(value string) (string, error) {
	if strings.ContainsRune(value, '\x00') {
		return "", errors.New("workspace path contains a null byte")
	}
	if value == "" || value == "." {
		return ".", nil
	}
	if strings.TrimSpace(value) != value {
		return "", errors.New("workspace path must not have surrounding whitespace")
	}
	if strings.Contains(value, "\\") || strings.HasPrefix(value, "//") {
		return "", errors.New("workspace path must use portable relative separators")
	}
	if path.IsAbs(value) || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return "", errors.New("workspace path must be relative")
	}
	if len(value) >= 2 && value[1] == ':' && isASCIILetter(value[0]) {
		return "", errors.New("workspace path must not contain a Windows drive")
	}
	cleaned := path.Clean(value)
	if cleaned != value {
		return "", errors.New("workspace path must be canonical")
	}
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("workspace path contains an unsafe segment")
		}
		if strings.TrimSpace(segment) != segment || strings.Contains(segment, ":") || strings.HasSuffix(segment, ".") {
			return "", fmt.Errorf("workspace path segment %q is not portable", segment)
		}
		if windowsReservedName.MatchString(segment) {
			return "", fmt.Errorf("workspace path segment %q is reserved on Windows", segment)
		}
	}
	return value, nil
}

func (s Service) List(
	rootPath string,
	taskID string,
	logicalPath string,
) ([]WorkspaceEntry, error) {
	root, normalizedPath, err := openWorkspaceRoot(rootPath, taskID, logicalPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if !isReadableWorkspacePath(normalizedPath) {
		return nil, errors.New("workspace path is private to the PI runtime")
	}
	resolvedPath, info, err := resolveExistingPath(root, normalizedPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("workspace path is not a directory")
	}
	entries, err := fs.ReadDir(root.FS(), resolvedPath)
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceEntry, 0, len(entries))
	for _, entry := range entries {
		entryPath := entry.Name()
		if resolvedPath != "." {
			entryPath = resolvedPath + "/" + entry.Name()
		}
		entryInfo, err := root.Lstat(entryPath)
		if err != nil {
			return nil, err
		}
		entryType := "file"
		readable := isReadableWorkspacePath(entryPath)
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			entryType = "symlink"
			readable = false
		} else if entryInfo.IsDir() {
			entryType = "directory"
		} else if !entryInfo.Mode().IsRegular() {
			entryType = "special"
			readable = false
		}
		result = append(result, WorkspaceEntry{
			Name:       entry.Name(),
			Path:       entryPath,
			Type:       entryType,
			ByteSize:   entryInfo.Size(),
			ModifiedAt: entryInfo.ModTime().UTC().Format(time.RFC3339Nano),
			Readable:   readable,
		})
	}
	sort.Slice(result, func(left int, right int) bool {
		if result[left].Type == result[right].Type {
			return result[left].Name < result[right].Name
		}
		if result[left].Type == "directory" {
			return true
		}
		if result[right].Type == "directory" {
			return false
		}
		return result[left].Name < result[right].Name
	})
	return result, nil
}

func (s Service) Read(
	rootPath string,
	taskID string,
	logicalPath string,
) (FilePreview, error) {
	root, normalizedPath, err := openWorkspaceRoot(rootPath, taskID, logicalPath)
	if err != nil {
		return FilePreview{}, err
	}
	defer root.Close()
	if normalizedPath == "." || !isReadableWorkspacePath(normalizedPath) {
		return FilePreview{}, errors.New("workspace file is not readable")
	}
	resolvedPath, info, err := resolveExistingPath(root, normalizedPath)
	if err != nil {
		return FilePreview{}, err
	}
	if !info.Mode().IsRegular() {
		return FilePreview{}, errors.New("workspace path is not a regular file")
	}
	if info.Size() > maxPreviewBytes {
		return FilePreview{}, errors.New("workspace file exceeds the 2 MiB preview limit")
	}
	content, err := root.ReadFile(resolvedPath)
	if err != nil {
		return FilePreview{}, err
	}
	hash := sha256.Sum256(content)
	mimeType := mimeTypeFor(resolvedPath, content)
	preview := FilePreview{
		Path:      resolvedPath,
		Name:      path.Base(resolvedPath),
		MIMEType:  mimeType,
		ByteSize:  int64(len(content)),
		SHA256:    hex.EncodeToString(hash[:]),
		Truncated: false,
	}
	if strings.HasPrefix(mimeType, "image/") {
		preview.Kind = "image"
		preview.Content = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(content)
		return preview, nil
	}
	if !isUTF8Text(content) || !isTextPreviewPath(resolvedPath, mimeType) {
		return FilePreview{}, errors.New("workspace file type is not supported for preview")
	}
	preview.Kind = "text"
	preview.Content = string(content)
	return preview, nil
}

func openWorkspaceRoot(
	rootPath string,
	taskID string,
	logicalPath string,
) (*os.Root, string, error) {
	if !taskIDPattern.MatchString(taskID) {
		return nil, "", fmt.Errorf("invalid task id %q", taskID)
	}
	normalizedPath, err := ValidateLogicalPath(logicalPath)
	if err != nil {
		return nil, "", err
	}
	rootPath, err = validateRoot(rootPath)
	if err != nil {
		return nil, "", err
	}
	taskPath := filepath.Join(rootPath, taskID)
	info, err := os.Lstat(taskPath)
	if err != nil {
		return nil, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, "", errors.New("task workspace root is not a regular directory")
	}
	root, err := os.OpenRoot(taskPath)
	if err != nil {
		return nil, "", err
	}
	manifest, exists, err := readManifest(root)
	if err != nil || !exists {
		root.Close()
		if err != nil {
			return nil, "", err
		}
		return nil, "", ErrWorkspaceNotReady
	}
	if err := validateManifest(manifest, taskID); err != nil {
		root.Close()
		return nil, "", err
	}
	return root, normalizedPath, nil
}

var ErrWorkspaceNotReady = errors.New("task workspace is not ready")

func resolveExistingPath(root *os.Root, logicalPath string) (string, fs.FileInfo, error) {
	if logicalPath == "." {
		info, err := root.Lstat(".")
		return ".", info, err
	}
	segments := strings.Split(logicalPath, "/")
	current := "."
	for index, segment := range segments {
		entries, err := fs.ReadDir(root.FS(), current)
		if err != nil {
			return "", nil, err
		}
		foundExact := false
		caseCollision := ""
		for _, entry := range entries {
			if entry.Name() == segment {
				foundExact = true
				break
			}
			if strings.EqualFold(entry.Name(), segment) {
				caseCollision = entry.Name()
			}
		}
		if caseCollision != "" {
			return "", nil, fmt.Errorf(
				"workspace path case collision: requested %q, found %q",
				segment,
				caseCollision,
			)
		}
		if !foundExact {
			return "", nil, fs.ErrNotExist
		}
		if current == "." {
			current = segment
		} else {
			current += "/" + segment
		}
		info, err := root.Lstat(current)
		if err != nil {
			return "", nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("workspace path %q is a symbolic link", current)
		}
		if index < len(segments)-1 && !info.IsDir() {
			return "", nil, fmt.Errorf("workspace path %q is not a directory", current)
		}
		if index == len(segments)-1 {
			return current, info, nil
		}
	}
	return "", nil, fs.ErrNotExist
}

func isReadableWorkspacePath(logicalPath string) bool {
	return logicalPath != ".btask/pi-agent" &&
		!strings.HasPrefix(logicalPath, ".btask/pi-agent/") &&
		logicalPath != ".btask/pi-sessions" &&
		!strings.HasPrefix(logicalPath, ".btask/pi-sessions/")
}

func isTextPreviewPath(logicalPath string, mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") || mimeType == "application/json" {
		return true
	}
	switch strings.ToLower(path.Ext(logicalPath)) {
	case ".md", ".txt", ".json", ".yaml", ".yml", ".csv", ".log", ".diff", ".patch":
		return true
	default:
		return false
	}
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
