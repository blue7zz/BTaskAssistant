package storage

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
)

const (
	taskContextFilename = "context.json"
	rootMarkerFilename  = ".btaskassistant-root"
	taskMarkerFilename  = ".btaskassistant-task.json"
)

var (
	taskIDPattern    = regexp.MustCompile(`^task_[a-z0-9][a-z0-9_-]{0,127}$`)
	dataImagePattern = regexp.MustCompile(
		`data:image/(png|jpeg|jpg|webp|gif);base64,([A-Za-z0-9+/=\r\n]+)`,
	)
)

type persistedWorkspace struct {
	State struct {
		Tasks        []json.RawMessage `json:"tasks"`
		TrashedTasks []json.RawMessage `json:"trashedTasks"`
	} `json:"state"`
}

type taskMarker struct {
	ID string `json:"id"`
}

type persistedEvidence struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type persistedTaskContext struct {
	Evidence []persistedEvidence `json:"evidence"`
}

func decodeTaskContexts(payload string) (map[string][]byte, error) {
	snapshots, err := decodeTaskSnapshots(payload)
	if err != nil {
		return nil, err
	}
	contexts := make(map[string][]byte, len(snapshots))
	for taskID, snapshot := range snapshots {
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, snapshot.RawJSON, "", "  "); err != nil {
			return nil, fmt.Errorf("format task %q: %w", taskID, err)
		}
		formatted.WriteByte('\n')
		contexts[taskID] = formatted.Bytes()
	}
	return contexts, nil
}

func decodeTaskSnapshots(payload string) (map[string]taskspace.TaskSnapshot, error) {
	var workspace persistedWorkspace
	if err := json.Unmarshal([]byte(payload), &workspace); err != nil {
		return nil, err
	}

	snapshots := make(map[string]taskspace.TaskSnapshot)
	foldedIDs := make(map[string]string)
	add := func(task json.RawMessage, archived bool) error {
		var snapshot taskspace.TaskSnapshot
		if err := json.Unmarshal(task, &snapshot); err != nil {
			return fmt.Errorf("decode task identity: %w", err)
		}
		if !taskIDPattern.MatchString(snapshot.ID) {
			return fmt.Errorf("invalid task id %q", snapshot.ID)
		}
		folded := strings.ToLower(snapshot.ID)
		if existing, exists := foldedIDs[folded]; exists {
			return fmt.Errorf("duplicate task ids %q and %q", existing, snapshot.ID)
		}
		if snapshot.Revision < 1 {
			snapshot.Revision = 1
		}
		snapshot.Archived = archived
		snapshot.RawJSON = append(json.RawMessage(nil), task...)
		snapshots[snapshot.ID] = snapshot
		foldedIDs[folded] = snapshot.ID
		return nil
	}
	for _, task := range workspace.State.Tasks {
		if err := add(task, false); err != nil {
			return nil, err
		}
	}
	for _, task := range workspace.State.TrashedTasks {
		if err := add(task, true); err != nil {
			return nil, err
		}
	}
	return snapshots, nil
}

func syncTaskContexts(rootPath string, contexts map[string][]byte) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()

	if err := writeRootFile(root, rootMarkerFilename, []byte("BTaskAssistant task context root\n")); err != nil {
		return fmt.Errorf("write task context root marker: %w", err)
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	existingNames := make(map[string][]string)
	for _, entry := range entries {
		folded := strings.ToLower(entry.Name())
		existingNames[folded] = append(existingNames[folded], entry.Name())
	}

	taskIDs := make([]string, 0, len(contexts))
	for taskID := range contexts {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)

	for _, taskID := range taskIDs {
		for _, existing := range existingNames[strings.ToLower(taskID)] {
			if existing != taskID {
				return fmt.Errorf("task directory name collision: %q and %q", existing, taskID)
			}
		}
		if err := ensureTaskDirectory(root, taskID); err != nil {
			return err
		}
		if err := ensureTaskSubdirectory(root, filepath.Join(taskID, "files")); err != nil {
			return fmt.Errorf("create task files directory %q: %w", taskID, err)
		}
		if err := ensureTaskSubdirectory(root, filepath.Join(taskID, "images")); err != nil {
			return fmt.Errorf("create task images directory %q: %w", taskID, err)
		}
		if err := materializeTaskFiles(root, taskID, contexts[taskID]); err != nil {
			return fmt.Errorf("materialize task files %q: %w", taskID, err)
		}
		contextPath := filepath.Join(taskID, taskContextFilename)
		if err := writeRootFile(root, contextPath, contexts[taskID]); err != nil {
			return fmt.Errorf("write task context %q: %w", taskID, err)
		}
	}
	return nil
}

func ensureTaskDirectory(root *os.Root, taskID string) error {
	info, err := root.Lstat(taskID)
	if errors.Is(err, fs.ErrNotExist) {
		if err := root.Mkdir(taskID, 0o700); err != nil {
			return fmt.Errorf("create task directory %q: %w", taskID, err)
		}
		if err := writeTaskMarker(root, taskID); err != nil {
			_ = root.Remove(taskID)
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("task path %q is not a regular directory", taskID)
	}
	markerPath := filepath.Join(taskID, taskMarkerFilename)
	markerInfo, err := root.Lstat(markerPath)
	if errors.Is(err, fs.ErrNotExist) {
		if err := adoptLegacyTaskDirectory(root, taskID); err != nil {
			return err
		}
		return writeTaskMarker(root, taskID)
	}
	if err != nil || markerInfo.Mode()&os.ModeSymlink != 0 || !markerInfo.Mode().IsRegular() {
		return fmt.Errorf("task directory %q is not managed by BTaskAssistant", taskID)
	}
	markerContent, err := root.ReadFile(markerPath)
	if err != nil {
		return fmt.Errorf("task directory %q is not managed by BTaskAssistant", taskID)
	}
	var marker taskMarker
	if err := json.Unmarshal(markerContent, &marker); err != nil || marker.ID != taskID {
		return fmt.Errorf("task directory %q has an invalid ownership marker", taskID)
	}
	return nil
}

func writeTaskMarker(root *os.Root, taskID string) error {
	marker, err := json.Marshal(taskMarker{ID: taskID})
	if err != nil {
		return err
	}
	marker = append(marker, '\n')
	return writeRootFile(root, filepath.Join(taskID, taskMarkerFilename), marker)
}

func adoptLegacyTaskDirectory(root *os.Root, taskID string) error {
	entries, err := fs.ReadDir(root.FS(), taskID)
	if err != nil {
		return fmt.Errorf("read legacy task directory %q: %w", taskID, err)
	}
	if len(entries) != 1 || entries[0].Name() != taskContextFilename {
		return fmt.Errorf("task directory %q is not managed by BTaskAssistant", taskID)
	}

	contextPath := filepath.Join(taskID, taskContextFilename)
	contextInfo, err := root.Lstat(contextPath)
	if err != nil || contextInfo.Mode()&os.ModeSymlink != 0 || !contextInfo.Mode().IsRegular() {
		return fmt.Errorf("task directory %q has an invalid legacy context", taskID)
	}
	content, err := root.ReadFile(contextPath)
	if err != nil {
		return fmt.Errorf("read legacy task context %q: %w", taskID, err)
	}
	var legacy struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(content, &legacy); err != nil || legacy.ID != taskID {
		return fmt.Errorf("task directory %q has an invalid legacy context", taskID)
	}
	return nil
}

func ensureTaskSubdirectory(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return root.Mkdir(name, 0o700)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%q is not a regular directory", name)
	}
	return nil
}

func writeRootFile(root *os.Root, name string, content []byte) error {
	info, err := root.Lstat(name)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%q is not a regular file", name)
		}
		existing, err := root.ReadFile(name)
		if err != nil {
			return err
		}
		if bytes.Equal(existing, content) {
			return nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	temporaryName, err := temporaryRootFilename(name)
	if err != nil {
		return err
	}
	temporary, err := root.OpenFile(
		temporaryName,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = root.Remove(temporaryName)
		}
	}()
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := root.Rename(temporaryName, name); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func temporaryRootFilename(name string) (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	directory := filepath.Dir(name)
	base := filepath.Base(name)
	return filepath.Join(directory, "."+base+"."+hex.EncodeToString(random)+".tmp"), nil
}

func materializeTaskFiles(root *os.Root, taskID string, context []byte) error {
	var task persistedTaskContext
	if err := json.Unmarshal(context, &task); err != nil {
		return err
	}
	for _, evidence := range task.Evidence {
		if evidence.Type != "file" || isImageFilename(evidence.Title) {
			continue
		}
		extension := safeTextExtension(evidence.Title)
		sum := sha256.Sum256([]byte(evidence.ID))
		name := hex.EncodeToString(sum[:8]) + extension
		if err := writeRootFile(
			root,
			filepath.Join(taskID, "files", name),
			[]byte(evidence.Content),
		); err != nil {
			return err
		}
	}

	var decoded any
	if err := json.Unmarshal(context, &decoded); err != nil {
		return err
	}
	images := make(map[string][]byte)
	collectDataImages(decoded, images)
	for name, content := range images {
		if err := writeRootFile(
			root,
			filepath.Join(taskID, "images", name),
			content,
		); err != nil {
			return err
		}
	}
	return nil
}

func isImageFilename(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func safeTextExtension(name string) string {
	extension := strings.ToLower(filepath.Ext(name))
	switch extension {
	case ".txt", ".md", ".json", ".yaml", ".yml", ".csv":
		return extension
	default:
		return ".txt"
	}
}

func collectDataImages(value any, images map[string][]byte) {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			collectDataImages(child, images)
		}
	case []any:
		for _, child := range typed {
			collectDataImages(child, images)
		}
	case string:
		for _, match := range dataImagePattern.FindAllStringSubmatch(typed, -1) {
			encoded := strings.NewReplacer("\r", "", "\n", "").Replace(match[2])
			content, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				continue
			}
			extension := strings.ToLower(match[1])
			if extension == "jpeg" {
				extension = "jpg"
			}
			sum := sha256.Sum256(content)
			images[hex.EncodeToString(sum[:])+"."+extension] = content
		}
	}
}
