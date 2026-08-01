package taskspace

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

func (s Service) ImportAttachments(
	rootPath string,
	taskID string,
	inputs []AttachmentInput,
) ([]Resource, error) {
	if len(inputs) == 0 {
		return []Resource{}, nil
	}
	if len(inputs) > MaxAttachmentCount {
		return nil, fmt.Errorf("a message may attach at most %d files", MaxAttachmentCount)
	}
	root, _, err := openWorkspaceRoot(rootPath, taskID, ".")
	if err != nil {
		return nil, err
	}
	defer root.Close()
	resources, err := readResourcesMirror(root, taskID)
	if err != nil {
		return nil, err
	}

	var total int64
	for _, input := range inputs {
		total += int64(len(input.Content))
		if total > MaxAttachmentBatchBytes {
			return nil, fmt.Errorf("attachment batch exceeds the %d MiB limit", MaxAttachmentBatchBytes/(1024*1024))
		}
	}

	result := make([]Resource, 0, len(inputs))
	changed := false
	for _, input := range inputs {
		resource, created, err := importAttachment(root, taskID, resources, input, s.now().UTC().Format(timeFormat))
		if err != nil {
			return nil, err
		}
		result = append(result, resource)
		if created {
			resources = append(resources, resource)
			changed = true
		}
	}
	if changed {
		if err := writeResourcesMirror(root, taskID, resources); err != nil {
			return nil, err
		}
		if err := bumpManifest(root, taskID, s.now().UTC().Format(timeFormat)); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func importAttachment(
	root *os.Root,
	taskID string,
	resources []Resource,
	input AttachmentInput,
	createdAt string,
) (Resource, bool, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || strings.ContainsAny(name, "\x00/\\") || name != filepath.Base(name) {
		return Resource{}, false, errors.New("attachment name must be a plain filename")
	}
	if len(input.Content) == 0 {
		return Resource{}, false, errors.New("attachment is empty")
	}
	if len(input.Content) > MaxAttachmentBytes {
		return Resource{}, false, fmt.Errorf("attachment %q exceeds the 16 MiB limit", name)
	}

	kind, extension, mimeType, err := classifyAttachment(name, input.MIMEType, input.Content)
	if err != nil {
		return Resource{}, false, fmt.Errorf("attachment %q: %w", name, err)
	}
	hash := sha256.Sum256(input.Content)
	hashText := hex.EncodeToString(hash[:])
	for _, resource := range resources {
		if resource.Kind == "attachment" && resource.SHA256 == hashText && resource.Readable {
			return resource, false, nil
		}
	}
	directory := "attachments/documents"
	if kind == "image" {
		directory = "attachments/images"
	}
	logicalPath := directory + "/" + safeStem(name, "attachment") + "-" + hashText[:12] + extension
	_, err = writeImmutableFile(root, logicalPath, input.Content)
	if err != nil {
		return Resource{}, false, err
	}
	resource := Resource{
		ID:          resourceID(taskID, logicalPath),
		TaskID:      taskID,
		Kind:        "attachment",
		SourceType:  "message_attachment",
		LogicalPath: logicalPath,
		StoragePath: logicalPath,
		MIMEType:    mimeType,
		ByteSize:    int64(len(input.Content)),
		SHA256:      hashText,
		Immutable:   true,
		Readable:    true,
		CreatedAt:   createdAt,
	}
	return resource, true, nil
}

func classifyAttachment(name string, declared string, content []byte) (string, string, string, error) {
	detected := http.DetectContentType(content)
	declared = strings.ToLower(strings.TrimSpace(strings.Split(declared, ";")[0]))
	switch detected {
	case "image/png":
		return validateDeclaredImage(declared, "image/png", ".png")
	case "image/jpeg":
		return validateDeclaredImage(declared, "image/jpeg", ".jpg")
	case "image/gif":
		return validateDeclaredImage(declared, "image/gif", ".gif")
	case "image/webp":
		return validateDeclaredImage(declared, "image/webp", ".webp")
	}

	extension := strings.ToLower(filepath.Ext(name))
	if isUTF8Text(content) {
		switch extension {
		case ".txt", ".md", ".json", ".yaml", ".yml", ".csv", ".tsv", ".xml":
		default:
			return "", "", "", errors.New("text documents require a supported text extension")
		}
		mimeType := mimeTypeFor(name, content)
		if declared != "" && declared != "application/octet-stream" && !strings.HasPrefix(declared, "text/") &&
			!(extension == ".json" && declared == "application/json") &&
			!(extension == ".xml" && (declared == "application/xml" || declared == "text/xml")) {
			return "", "", "", fmt.Errorf("declared MIME %q does not match text content", declared)
		}
		return "document", extension, mimeType, nil
	}
	if bytes.HasPrefix(content, []byte("%PDF-")) && extension == ".pdf" {
		if declared != "" && declared != "application/pdf" && declared != "application/octet-stream" {
			return "", "", "", fmt.Errorf("declared MIME %q does not match PDF content", declared)
		}
		return "document", extension, "application/pdf", nil
	}
	if officeMIME, ok := validOfficeDocument(extension, content); ok {
		if declared != "" && declared != officeMIME && declared != "application/octet-stream" {
			return "", "", "", fmt.Errorf("declared MIME %q does not match Office document", declared)
		}
		return "document", extension, officeMIME, nil
	}
	return "", "", "", errors.New("unsupported attachment MIME or file signature")
}

func validateDeclaredImage(declared string, detected string, extension string) (string, string, string, error) {
	if declared != "" && declared != detected && declared != "application/octet-stream" {
		return "", "", "", fmt.Errorf("declared MIME %q does not match %s content", declared, detected)
	}
	return "image", extension, detected, nil
}

func validOfficeDocument(extension string, content []byte) (string, bool) {
	var prefix, mimeType string
	switch extension {
	case ".docx":
		prefix = "word/"
		mimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		prefix = "xl/"
		mimeType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		prefix = "ppt/"
		mimeType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		return "", false
	}
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", false
	}
	hasContentTypes := false
	hasPayload := false
	for _, file := range reader.File {
		if file.Name == "[Content_Types].xml" {
			hasContentTypes = true
		}
		if strings.HasPrefix(file.Name, prefix) {
			hasPayload = true
		}
	}
	return mimeType, hasContentTypes && hasPayload
}

func (s Service) WriteArtifact(
	rootPath string,
	taskID string,
	kind string,
	name string,
	content []byte,
) (ArtifactFile, error) {
	root, _, err := openWorkspaceRoot(rootPath, taskID, ".")
	if err != nil {
		return ArtifactFile{}, err
	}
	defer root.Close()
	logicalPath, err := artifactLogicalPath(kind, name)
	if err != nil {
		return ArtifactFile{}, err
	}
	if len(content) == 0 || len(content) > MaxArtifactBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return ArtifactFile{}, errors.New("artifact must be non-empty UTF-8 text no larger than 2 MiB")
	}
	if _, err := writeAtomicFile(root, logicalPath, content); err != nil {
		return ArtifactFile{}, err
	}
	if err := bumpManifest(root, taskID, s.now().UTC().Format(timeFormat)); err != nil {
		return ArtifactFile{}, err
	}
	hash := sha256.Sum256(content)
	return ArtifactFile{
		LogicalPath: logicalPath,
		Kind:        kind,
		MIMEType:    mimeTypeFor(logicalPath, content),
		ByteSize:    int64(len(content)),
		SHA256:      hex.EncodeToString(hash[:]),
	}, nil
}

func artifactLogicalPath(kind string, name string) (string, error) {
	directory := map[string]string{
		"plan": "plans", "report": "reports", "proposal": "proposals", "export": "exports",
	}[strings.ToLower(strings.TrimSpace(kind))]
	if directory == "" {
		return "", errors.New("unsupported artifact kind")
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, "\x00/\\") || name != filepath.Base(name) {
		return "", errors.New("artifact name must be a plain filename")
	}
	extension := strings.ToLower(filepath.Ext(name))
	switch extension {
	case "":
		name += ".md"
	case ".md", ".txt", ".json", ".yaml", ".yml", ".csv":
	default:
		return "", errors.New("artifact extension is not supported")
	}
	logicalPath := "artifacts/" + directory + "/" + name
	if normalized, err := ValidateLogicalPath(logicalPath); err != nil || normalized != logicalPath {
		return "", errors.New("artifact path is unsafe")
	}
	return logicalPath, nil
}

func (s Service) InstallAgentExtension(
	rootPath string,
	taskID string,
	filename string,
	content []byte,
) (string, error) {
	root, _, err := openWorkspaceRoot(rootPath, taskID, ".")
	if err != nil {
		return "", err
	}
	defer root.Close()
	if filename == "" || strings.ContainsAny(filename, "\x00/\\") || filepath.Ext(filename) != ".ts" {
		return "", errors.New("PI extension filename is invalid")
	}
	logicalPath := ".btask/pi-agent/" + filename
	if _, err := writeAtomicFile(root, logicalPath, content); err != nil {
		return "", err
	}
	return filepath.Join(rootPath, taskID, filepath.FromSlash(logicalPath)), nil
}

func (s Service) ReadBytes(
	rootPath string,
	taskID string,
	logicalPath string,
	limit int64,
) ([]byte, string, error) {
	root, normalizedPath, err := openWorkspaceRoot(rootPath, taskID, logicalPath)
	if err != nil {
		return nil, "", err
	}
	defer root.Close()
	if normalizedPath == "." || !isReadableWorkspacePath(normalizedPath) {
		return nil, "", errors.New("workspace file is not readable")
	}
	resolvedPath, info, err := resolveExistingPath(root, normalizedPath)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("workspace path is not a regular file")
	}
	if limit <= 0 || info.Size() > limit {
		return nil, "", errors.New("workspace file exceeds the read limit")
	}
	content, err := root.ReadFile(resolvedPath)
	if err != nil {
		return nil, "", err
	}
	return content, mimeTypeFor(resolvedPath, content), nil
}

func writeResourcesMirror(root *os.Root, taskID string, resources []Resource) error {
	sort.Slice(resources, func(left int, right int) bool {
		return resources[left].LogicalPath < resources[right].LogicalPath
	})
	content, err := json.MarshalIndent(resourcesMirror{
		SchemaVersion: SchemaVersion,
		TaskID:        taskID,
		Resources:     resources,
	}, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	_, err = writeAtomicFile(root, ".btask/resources.json", content)
	return err
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func bumpManifest(root *os.Root, taskID string, updatedAt string) error {
	manifest, exists, err := readManifest(root)
	if err != nil || !exists {
		if err != nil {
			return err
		}
		return ErrWorkspaceNotReady
	}
	if err := validateManifest(manifest, taskID); err != nil {
		return err
	}
	manifest.Revision++
	manifest.UpdatedAt = updatedAt
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	_, err = writeAtomicFile(root, ".btask/manifest.json", content)
	return err
}
