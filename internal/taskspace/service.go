package taskspace

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
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	legacyTaskMarker = ".btaskassistant-task.json"
	legacyContext    = "context.json"
)

var (
	taskIDPattern    = regexp.MustCompile(`^task_[a-z0-9][a-z0-9_-]{0,127}$`)
	dataImagePattern = regexp.MustCompile(
		`data:image/([A-Za-z0-9.+-]+);base64,([A-Za-z0-9+/=\r\n]*)`,
	)
	windowsReservedName = regexp.MustCompile(
		`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$`,
	)
)

var managedDirectories = []string{
	".btask",
	".btask/pi-agent",
	".btask/pi-sessions",
	"context",
	"context/requirements",
	"sources",
	"sources/manual",
	"sources/plane",
	"sources/chats",
	"sources/project-observations",
	"attachments",
	"attachments/images",
	"attachments/documents",
	"artifacts",
	"artifacts/plans",
	"artifacts/reports",
	"artifacts/proposals",
	"artifacts/exports",
	"repos",
	"runs",
}

type Service struct {
	Now func() time.Time
}

func (s Service) Ensure(rootPath string, snapshot TaskSnapshot) (Result, error) {
	if !taskIDPattern.MatchString(snapshot.ID) {
		return Result{}, fmt.Errorf("invalid task id %q", snapshot.ID)
	}
	rootPath, err := validateRoot(rootPath)
	if err != nil {
		return Result{}, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return Result{}, fmt.Errorf("open task data root: %w", err)
	}
	defer root.Close()
	taskDirectoryChanged, err := ensureTaskDirectory(root, snapshot.ID)
	if err != nil {
		return Result{}, err
	}
	taskRoot, err := root.OpenRoot(snapshot.ID)
	if err != nil {
		return Result{}, fmt.Errorf("open task workspace %q: %w", snapshot.ID, err)
	}
	defer taskRoot.Close()
	directoriesChanged := taskDirectoryChanged
	for _, directory := range managedDirectories {
		created, err := ensureDirectory(taskRoot, directory)
		if err != nil {
			return Result{}, fmt.Errorf("ensure task workspace directory %q: %w", directory, err)
		}
		directoriesChanged = directoriesChanged || created
	}

	now := s.now().UTC().Format(time.RFC3339Nano)
	manifest, manifestExists, err := readManifest(taskRoot)
	if err != nil {
		return Result{}, err
	}
	if !manifestExists {
		workspaceID, err := newWorkspaceID()
		if err != nil {
			return Result{}, err
		}
		manifest = Manifest{
			SchemaVersion:    SchemaVersion,
			TaskID:           snapshot.ID,
			WorkspaceID:      workspaceID,
			CreatedAt:        stableTime(snapshot.CreatedAt, now),
			UpdatedAt:        now,
			Revision:         1,
			Engine:           "pi",
			ResourcePolicy:   ResourcePolicy,
			PIResourcePolicy: ResourcePolicy,
			ManagedBy:        ManagedBy,
		}
	} else if err := validateManifest(manifest, snapshot.ID); err != nil {
		return Result{}, err
	}
	previousResources := []Resource{}
	if manifestExists {
		previousResources, err = readResourcesMirror(taskRoot, snapshot.ID)
		if err != nil {
			return Result{}, err
		}
	}

	changed := directoriesChanged
	resources := make([]Resource, 0)
	resourcePaths := make(map[string]struct{})
	attachmentByHash := make(map[string]string)
	addResource := func(resource Resource) error {
		if _, exists := resourcePaths[resource.LogicalPath]; exists {
			return nil
		}
		resourcePaths[resource.LogicalPath] = struct{}{}
		resources = append(resources, resource)
		return nil
	}
	writeManaged := func(
		logicalPath string,
		content []byte,
		kind string,
		sourceType string,
		immutable bool,
		createdAt string,
	) error {
		var fileChanged bool
		var err error
		if immutable {
			fileChanged, err = writeImmutableFile(taskRoot, logicalPath, content)
		} else {
			fileChanged, err = writeAtomicFile(taskRoot, logicalPath, content)
		}
		if err != nil {
			return err
		}
		changed = changed || fileChanged
		hash := sha256.Sum256(content)
		storagePath := logicalPath
		return addResource(Resource{
			ID:          resourceID(snapshot.ID, logicalPath),
			TaskID:      snapshot.ID,
			Kind:        kind,
			SourceType:  sourceType,
			LogicalPath: logicalPath,
			StoragePath: storagePath,
			MIMEType:    mimeTypeFor(logicalPath, content),
			ByteSize:    int64(len(content)),
			SHA256:      hex.EncodeToString(hash[:]),
			Immutable:   immutable,
			Readable:    true,
			CreatedAt:   stableTime(createdAt, manifest.CreatedAt),
		})
	}
	for _, resource := range previousResources {
		if !strings.HasPrefix(resource.SourceType, "legacy_") {
			continue
		}
		if err := validateRetainedLegacyResource(resource, snapshot.ID); err != nil {
			return Result{}, err
		}
		content, err := taskRoot.ReadFile(resource.LogicalPath)
		if err != nil {
			return Result{}, fmt.Errorf("read retained legacy resource %q: %w", resource.LogicalPath, err)
		}
		hash := sha256.Sum256(content)
		if resource.SHA256 != hex.EncodeToString(hash[:]) || resource.ByteSize != int64(len(content)) {
			return Result{}, fmt.Errorf("retained legacy resource %q no longer matches its hash", resource.LogicalPath)
		}
		if resource.SourceType == "legacy_image" {
			switch http.DetectContentType(content) {
			case "image/png", "image/jpeg", "image/gif", "image/webp":
			default:
				return Result{}, fmt.Errorf("retained legacy resource %q is not a supported image", resource.LogicalPath)
			}
		}
		if err := addResource(resource); err != nil {
			return Result{}, err
		}
		if resource.Kind == "attachment" {
			key := filepath.ToSlash(filepath.Dir(resource.LogicalPath)) + ":" + resource.SHA256
			attachmentByHash[key] = resource.LogicalPath
		}
	}

	contextFiles := []struct {
		path       string
		content    string
		sourceType string
	}{
		{"context/task.md", renderTask(snapshot), "task"},
		{"context/requirements/current.md", renderCurrentRequirements(snapshot), "requirements"},
		{"context/acceptance-criteria.md", renderAcceptanceCriteria(snapshot), "acceptance_criteria"},
	}
	for _, file := range contextFiles {
		if err := writeManaged(
			file.path,
			[]byte(file.content),
			"context",
			file.sourceType,
			false,
			snapshot.CreatedAt,
		); err != nil {
			return Result{}, fmt.Errorf("write %s: %w", file.path, err)
		}
	}

	approved := append([]ApprovedRequirementRevision(nil), snapshot.Requirements.ApprovedRevisions...)
	sort.Slice(approved, func(left int, right int) bool {
		return approved[left].Version < approved[right].Version
	})
	seenVersions := make(map[int]struct{}, len(approved))
	for _, revision := range approved {
		if revision.Version < 1 {
			return Result{}, errors.New("approved requirement revision must be positive")
		}
		if _, exists := seenVersions[revision.Version]; exists {
			return Result{}, fmt.Errorf("duplicate approved requirement revision %d", revision.Version)
		}
		seenVersions[revision.Version] = struct{}{}
		logicalPath := "context/requirements/approved-v" + strconv.Itoa(revision.Version) + ".md"
		if err := writeManaged(
			logicalPath,
			[]byte(renderApprovedRequirement(revision)),
			"context",
			"approved_requirement",
			true,
			revision.ConfirmedAt,
		); err != nil {
			return Result{}, fmt.Errorf("preserve approved requirement v%d: %w", revision.Version, err)
		}
	}

	addAttachment := func(
		content []byte,
		title string,
		sourceType string,
		image bool,
		forcedExtension string,
		createdAt string,
	) error {
		if len(content) > MaxAttachmentBytes {
			return fmt.Errorf("attachment %q exceeds the 16 MiB limit", title)
		}
		if image {
			detectedMIME := http.DetectContentType(content)
			switch detectedMIME {
			case "image/png", "image/jpeg", "image/gif", "image/webp":
			default:
				return fmt.Errorf(
					"attachment %q is not a supported PNG, JPEG, GIF, or WebP image",
					title,
				)
			}
		}
		hash := sha256.Sum256(content)
		hashText := hex.EncodeToString(hash[:])
		directory := "attachments/documents"
		if image {
			directory = "attachments/images"
		}
		key := directory + ":" + hashText
		if existing, exists := attachmentByHash[key]; exists {
			return addResource(Resource{
				ID:          resourceID(snapshot.ID, existing),
				TaskID:      snapshot.ID,
				Kind:        "attachment",
				SourceType:  sourceType,
				LogicalPath: existing,
				StoragePath: existing,
				MIMEType:    mimeTypeFor(existing, content),
				ByteSize:    int64(len(content)),
				SHA256:      hashText,
				Immutable:   true,
				Readable:    true,
				CreatedAt:   stableTime(createdAt, manifest.CreatedAt),
			})
		}
		extension := forcedExtension
		if extension == "" {
			extension = safeExtension(title, content, image)
		}
		base := safeStem(title, "attachment")
		logicalPath := directory + "/" + base + "-" + hashText[:12] + extension
		attachmentByHash[key] = logicalPath
		return writeManaged(
			logicalPath,
			content,
			"attachment",
			sourceType,
			true,
			createdAt,
		)
	}

	warnings := make([]string, 0)
	warnings = appendEngineIdentityWarning(
		warnings,
		"development.engine",
		snapshot.Development.Engine,
	)
	warnings = appendEngineIdentityWarning(
		warnings,
		"requirements.interview.analyst",
		snapshot.Requirements.Interview.Analyst,
	)
	for _, evidence := range snapshot.Evidence {
		if evidence.ID == "" {
			return Result{}, errors.New("task evidence id is required")
		}
		if evidence.Type == "file" {
			images, err := decodeDataImages(evidence.Content)
			if err != nil {
				return Result{}, fmt.Errorf("decode evidence %q images: %w", evidence.Title, err)
			}
			for _, image := range images {
				if err := addAttachment(
					image.Content,
					evidence.Title,
					"image",
					true,
					image.Extension,
					evidence.CreatedAt,
				); err != nil {
					return Result{}, err
				}
			}
			if len(images) == 0 || !looksLikeImageFilename(evidence.Title) {
				if err := addAttachment(
					[]byte(evidence.Content),
					evidence.Title,
					"document",
					false,
					"",
					evidence.CreatedAt,
				); err != nil {
					return Result{}, err
				}
			}
			continue
		}
		directory, sourceType := sourceDirectory(evidence.Type)
		if directory == "sources/manual" && evidence.Type != "manual" {
			warnings = append(warnings, "未知来源类型 "+evidence.Type+" 已按 manual 导入")
		}
		content := []byte(evidence.Content)
		hash := sha256.Sum256(content)
		extension := safeTextExtension(evidence.Title)
		logicalPath := directory + "/" + safeStem(evidence.Title, evidence.ID) + "-" + hex.EncodeToString(hash[:6]) + extension
		if err := writeManaged(
			logicalPath,
			content,
			"source",
			sourceType,
			true,
			evidence.CreatedAt,
		); err != nil {
			return Result{}, fmt.Errorf("import evidence %q: %w", evidence.Title, err)
		}
	}

	for _, observation := range snapshot.Requirements.Interview.ProjectObservations {
		content := renderProjectObservation(observation)
		hash := sha256.Sum256([]byte(content))
		logicalPath := "sources/project-observations/" + safeStem(observation.FilePath, observation.ID) + "-" + hex.EncodeToString(hash[:6]) + ".md"
		if err := writeManaged(
			logicalPath,
			[]byte(content),
			"source",
			"project_observation",
			true,
			snapshot.UpdatedAt,
		); err != nil {
			return Result{}, fmt.Errorf("import project observation: %w", err)
		}
	}

	raw := snapshot.RawJSON
	if len(raw) == 0 {
		raw, err = json.Marshal(snapshot)
		if err != nil {
			return Result{}, err
		}
	}
	images, err := decodeDataImages(string(raw))
	if err != nil {
		return Result{}, fmt.Errorf("decode task data images: %w", err)
	}
	for _, image := range images {
		if err := addAttachment(
			image.Content,
			"embedded-image"+image.Extension,
			"image",
			true,
			image.Extension,
			snapshot.CreatedAt,
		); err != nil {
			return Result{}, err
		}
	}

	migrationStartedAt := now
	legacyContextPath := ""
	if info, statErr := taskRoot.Lstat(legacyContext); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return Result{}, errors.New("legacy context.json is not a regular file")
		}
		if info.Size() > MaxAttachmentBytes {
			return Result{}, errors.New("legacy context.json exceeds the 16 MiB limit")
		}
		legacyContextPath = filepath.Join(rootPath, snapshot.ID, legacyContext)
		if !manifestExists {
			legacyContent, err := taskRoot.ReadFile(legacyContext)
			if err != nil {
				return Result{}, fmt.Errorf("read legacy context.json: %w", err)
			}
			legacyHash := sha256.Sum256(legacyContent)
			legacyLogicalPath := "sources/manual/legacy-context-" + hex.EncodeToString(legacyHash[:6]) + ".json"
			if err := writeManaged(
				legacyLogicalPath,
				legacyContent,
				"source",
				"legacy_context",
				true,
				snapshot.CreatedAt,
			); err != nil {
				return Result{}, fmt.Errorf("copy legacy context.json: %w", err)
			}
		}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return Result{}, statErr
	}
	for _, legacyDirectory := range []struct {
		name       string
		sourceType string
		image      bool
	}{
		{"files", "legacy_document", false},
		{"images", "legacy_image", true},
	} {
		err := walkLegacyFiles(taskRoot, legacyDirectory.name, func(name string, content []byte) error {
			return addAttachment(
				content,
				filepath.Base(name),
				legacyDirectory.sourceType,
				legacyDirectory.image,
				"",
				snapshot.CreatedAt,
			)
		})
		if err != nil {
			return Result{}, fmt.Errorf("migrate legacy %s: %w", legacyDirectory.name, err)
		}
	}

	sort.Slice(resources, func(left int, right int) bool {
		return resources[left].LogicalPath < resources[right].LogicalPath
	})
	mirror, err := json.MarshalIndent(resourcesMirror{
		SchemaVersion: SchemaVersion,
		TaskID:        snapshot.ID,
		Resources:     resources,
	}, "", "  ")
	if err != nil {
		return Result{}, err
	}
	mirror = append(mirror, '\n')
	mirrorChanged, err := writeAtomicFile(taskRoot, ".btask/resources.json", mirror)
	if err != nil {
		return Result{}, fmt.Errorf("write resources mirror: %w", err)
	}
	changed = changed || mirrorChanged

	for path, content := range map[string][]byte{
		".btask/permissions.json": []byte(fmt.Sprintf(
			"{\n  \"schemaVersion\": 1,\n  \"taskId\": %q,\n  \"resourcePolicy\": \"isolated\",\n  \"grants\": []\n}\n",
			snapshot.ID,
		)),
		".btask/sessions.json": []byte(fmt.Sprintf(
			"{\n  \"schemaVersion\": 1,\n  \"taskId\": %q,\n  \"sessions\": []\n}\n",
			snapshot.ID,
		)),
	} {
		created, err := createFileIfMissing(taskRoot, path, content)
		if err != nil {
			return Result{}, err
		}
		changed = changed || created
	}

	if manifestExists && changed {
		manifest.Revision++
		manifest.UpdatedAt = now
	}
	manifestContent, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Result{}, err
	}
	manifestContent = append(manifestContent, '\n')
	manifestChanged, err := writeAtomicFile(taskRoot, ".btask/manifest.json", manifestContent)
	if err != nil {
		return Result{}, fmt.Errorf("write task workspace manifest: %w", err)
	}
	if !manifestExists && !manifestChanged {
		return Result{}, errors.New("task workspace manifest was not created")
	}

	return Result{
		TaskID:               snapshot.ID,
		WorkspaceID:          manifest.WorkspaceID,
		RootPath:             filepath.Join(rootPath, snapshot.ID),
		SchemaVersion:        manifest.SchemaVersion,
		ManifestRevision:     manifest.Revision,
		CreatedAt:            manifest.CreatedAt,
		UpdatedAt:            manifest.UpdatedAt,
		LegacyContextPath:    legacyContextPath,
		MigrationStartedAt:   migrationStartedAt,
		MigrationCompletedAt: now,
		Warnings:             uniqueStrings(warnings),
		Resources:            resources,
	}, nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func validateRoot(rootPath string) (string, error) {
	trimmed := strings.TrimSpace(rootPath)
	if trimmed == "" {
		return "", errors.New("task data root is required")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(absolute)
	info, err := os.Lstat(cleaned)
	if err != nil {
		return "", fmt.Errorf("task data root is unavailable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("task data root must be a regular directory")
	}
	return cleaned, nil
}

func ensureTaskDirectory(root *os.Root, taskID string) (bool, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), taskID) && entry.Name() != taskID {
			return false, fmt.Errorf("task directory name collision: %q and %q", entry.Name(), taskID)
		}
	}
	info, err := root.Lstat(taskID)
	if errors.Is(err, fs.ErrNotExist) {
		if err := root.Mkdir(taskID, 0o700); err != nil {
			return false, err
		}
		marker, _ := json.Marshal(struct {
			ID string `json:"id"`
		}{ID: taskID})
		marker = append(marker, '\n')
		taskRoot, openErr := root.OpenRoot(taskID)
		if openErr != nil {
			return false, openErr
		}
		defer taskRoot.Close()
		_, writeErr := writeAtomicFile(taskRoot, legacyTaskMarker, marker)
		return true, writeErr
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, fmt.Errorf("task path %q is not a regular directory", taskID)
	}
	changed := false
	if info.Mode().Perm() != 0o700 {
		if err := root.Chmod(taskID, 0o700); err != nil {
			return false, err
		}
		changed = true
	}
	markerPath := filepath.Join(taskID, legacyTaskMarker)
	markerInfo, markerErr := root.Lstat(markerPath)
	manifestPath := filepath.Join(taskID, ".btask", "manifest.json")
	manifestInfo, manifestErr := root.Lstat(manifestPath)
	if markerErr != nil && manifestErr != nil {
		return false, fmt.Errorf("task directory %q is not managed by BTaskAssistant", taskID)
	}
	if markerErr == nil && (markerInfo.Mode()&os.ModeSymlink != 0 || !markerInfo.Mode().IsRegular()) {
		return false, fmt.Errorf("task directory %q has an invalid ownership marker", taskID)
	}
	if markerErr == nil {
		if markerInfo.Mode().Perm() != 0o600 {
			if err := root.Chmod(markerPath, 0o600); err != nil {
				return false, err
			}
			changed = true
		}
		content, err := root.ReadFile(markerPath)
		if err != nil {
			return false, fmt.Errorf("read task directory %q ownership marker: %w", taskID, err)
		}
		var marker struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(content, &marker); err != nil || marker.ID != taskID {
			return false, fmt.Errorf("task directory %q has an invalid ownership marker", taskID)
		}
	} else if !errors.Is(markerErr, fs.ErrNotExist) {
		return false, markerErr
	}
	if manifestErr == nil && (manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular()) {
		return false, fmt.Errorf("task directory %q has an invalid manifest", taskID)
	}
	if manifestErr != nil && !errors.Is(manifestErr, fs.ErrNotExist) {
		return false, manifestErr
	}
	return changed, nil
}

func ensureDirectory(root *os.Root, name string) (bool, error) {
	changed := false
	current := ""
	for _, segment := range strings.Split(filepath.ToSlash(name), "/") {
		parent := "."
		if current != "" {
			parent = current
		}
		if err := rejectCaseCollision(root, parent, segment); err != nil {
			return false, err
		}
		if current == "" {
			current = segment
		} else {
			current = filepath.Join(current, segment)
		}
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if err := root.Mkdir(current, 0o700); err != nil {
				return false, err
			}
			if err := root.Chmod(current, 0o700); err != nil {
				return false, err
			}
			changed = true
			continue
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, fmt.Errorf("%q is not a regular directory", current)
		}
		if info.Mode().Perm() != 0o700 {
			if err := root.Chmod(current, 0o700); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

func readManifest(root *os.Root) (Manifest, bool, error) {
	info, err := root.Lstat(".btask/manifest.json")
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Manifest{}, false, errors.New("task workspace manifest is not a regular file")
	}
	content, err := root.ReadFile(".btask/manifest.json")
	if err != nil {
		return Manifest{}, false, err
	}
	var manifest Manifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return Manifest{}, false, fmt.Errorf("decode task workspace manifest: %w", err)
	}
	return manifest, true, nil
}

func readResourcesMirror(root *os.Root, taskID string) ([]Resource, error) {
	info, err := root.Lstat(".btask/resources.json")
	if err != nil {
		return nil, fmt.Errorf("read resources mirror metadata: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("task workspace resources mirror is not a regular file")
	}
	content, err := root.ReadFile(".btask/resources.json")
	if err != nil {
		return nil, err
	}
	var mirror resourcesMirror
	if err := json.Unmarshal(content, &mirror); err != nil {
		return nil, fmt.Errorf("decode task workspace resources mirror: %w", err)
	}
	if mirror.SchemaVersion != SchemaVersion || mirror.TaskID != taskID {
		return nil, errors.New("task workspace resources mirror identity is invalid")
	}
	return mirror.Resources, nil
}

func validateRetainedLegacyResource(resource Resource, taskID string) error {
	if resource.TaskID != taskID || resource.StoragePath != resource.LogicalPath ||
		resource.ID != resourceID(taskID, resource.LogicalPath) ||
		!resource.Immutable || !resource.Readable || resource.ByteSize < 0 ||
		resource.CreatedAt == "" {
		return fmt.Errorf("retained legacy resource %q has invalid metadata", resource.LogicalPath)
	}
	logicalPath, err := ValidateLogicalPath(resource.LogicalPath)
	if err != nil || logicalPath == "." || !isReadableWorkspacePath(logicalPath) {
		return fmt.Errorf("retained legacy resource %q has an unsafe path", resource.LogicalPath)
	}
	validLocation := (resource.SourceType == "legacy_context" &&
		resource.Kind == "source" && strings.HasPrefix(logicalPath, "sources/manual/")) ||
		(resource.SourceType == "legacy_document" &&
			resource.Kind == "attachment" && strings.HasPrefix(logicalPath, "attachments/documents/")) ||
		(resource.SourceType == "legacy_image" &&
			resource.Kind == "attachment" && strings.HasPrefix(logicalPath, "attachments/images/"))
	if !validLocation {
		return fmt.Errorf("retained legacy resource %q is outside its managed location", resource.LogicalPath)
	}
	hash, err := hex.DecodeString(resource.SHA256)
	if err != nil || len(hash) != sha256.Size {
		return fmt.Errorf("retained legacy resource %q has an invalid hash", resource.LogicalPath)
	}
	return nil
}

func validateManifest(manifest Manifest, taskID string) error {
	if manifest.ManagedBy != ManagedBy || manifest.TaskID != taskID {
		return errors.New("task workspace manifest identity does not match its directory")
	}
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported task workspace schema version %d", manifest.SchemaVersion)
	}
	if manifest.WorkspaceID == "" || manifest.Revision < 1 {
		return errors.New("task workspace manifest is incomplete")
	}
	if manifest.Engine != "pi" || manifest.ResourcePolicy != ResourcePolicy || manifest.PIResourcePolicy != ResourcePolicy {
		return errors.New("task workspace PI resource policy is not isolated")
	}
	return nil
}

func writeAtomicFile(root *os.Root, name string, content []byte) (bool, error) {
	if err := rejectCaseCollision(root, filepath.Dir(name), filepath.Base(name)); err != nil {
		return false, err
	}
	info, err := root.Lstat(name)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return false, fmt.Errorf("%q is not a regular file", name)
		}
		existing, err := root.ReadFile(name)
		if err != nil {
			return false, err
		}
		if bytes.Equal(existing, content) {
			return ensureManagedFileMode(root, name, info)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	temporaryName, err := temporaryFilename(name)
	if err != nil {
		return false, err
	}
	temporary, err := root.OpenFile(temporaryName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = root.Remove(temporaryName)
		}
	}()
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return false, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := root.Rename(temporaryName, name); err != nil {
		return false, err
	}
	removeTemporary = false
	if err := root.Chmod(name, 0o600); err != nil {
		return false, err
	}
	if err := syncDirectory(root, filepath.Dir(name)); err != nil {
		return false, err
	}
	return true, nil
}

func rejectCaseCollision(root *os.Root, parent string, name string) error {
	entries, err := fs.ReadDir(root.FS(), parent)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != name && strings.EqualFold(entry.Name(), name) {
			return fmt.Errorf("workspace path case collision: %q and %q", entry.Name(), name)
		}
	}
	return nil
}

func writeImmutableFile(root *os.Root, name string, content []byte) (bool, error) {
	info, err := root.Lstat(name)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return false, fmt.Errorf("%q is not a regular file", name)
		}
		existing, err := root.ReadFile(name)
		if err != nil {
			return false, err
		}
		if !bytes.Equal(existing, content) {
			return false, fmt.Errorf("immutable file %q already exists with different content", name)
		}
		return ensureManagedFileMode(root, name, info)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return writeAtomicFile(root, name, content)
}

func createFileIfMissing(root *os.Root, name string, content []byte) (bool, error) {
	info, err := root.Lstat(name)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return false, fmt.Errorf("%q is not a regular file", name)
		}
		return ensureManagedFileMode(root, name, info)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return writeAtomicFile(root, name, content)
}

func ensureManagedFileMode(
	root *os.Root,
	name string,
	info fs.FileInfo,
) (bool, error) {
	if info.Mode().Perm() == 0o600 {
		return false, nil
	}
	if err := root.Chmod(name, 0o600); err != nil {
		return false, err
	}
	if err := syncDirectory(root, filepath.Dir(name)); err != nil {
		return false, err
	}
	return true, nil
}

func temporaryFilename(name string) (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return filepath.Join(
		filepath.Dir(name),
		"."+filepath.Base(name)+"."+hex.EncodeToString(random)+".tmp",
	), nil
}

func syncDirectory(root *os.Root, name string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func newWorkspaceID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}

func resourceID(taskID string, logicalPath string) string {
	hash := sha256.Sum256([]byte(taskID + "\x00" + logicalPath))
	return "resource_" + hex.EncodeToString(hash[:16])
}

func stableTime(value string, fallback string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value)); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	return fallback
}

func renderTask(task TaskSnapshot) string {
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = task.ID
	}
	var content strings.Builder
	fmt.Fprintf(&content, "# %s\n\n", title)
	fmt.Fprintf(&content, "- Task ID: `%s`\n", task.ID)
	if task.Status != "" {
		fmt.Fprintf(&content, "- 状态: `%s`\n", task.Status)
	}
	if task.Priority != "" {
		fmt.Fprintf(&content, "- 优先级: `%s`\n", task.Priority)
	}
	if task.ProjectName != "" {
		fmt.Fprintf(&content, "- 项目: %s\n", task.ProjectName)
	}
	if task.ProjectPath != "" {
		fmt.Fprintf(&content, "- 项目路径: `%s`\n", task.ProjectPath)
	}
	fmt.Fprintf(&content, "- 任务修订: %d\n", task.Revision)
	content.WriteString("\n## 摘要\n\n")
	if strings.TrimSpace(task.Summary) == "" {
		content.WriteString("暂无摘要。\n")
	} else {
		content.WriteString(strings.TrimSpace(task.Summary) + "\n")
	}
	return content.String()
}

func renderCurrentRequirements(task TaskSnapshot) string {
	var content strings.Builder
	if document := strings.TrimSpace(task.Requirements.Document); document != "" {
		content.WriteString(document + "\n")
	} else {
		content.WriteString("# 当前需求\n\n")
		writeMarkdownSection(&content, "目标", []string{task.Requirements.Objective})
		writeMarkdownSection(&content, "范围", task.Requirements.Scope)
		writeMarkdownSection(&content, "不在范围", task.Requirements.OutOfScope)
		writeMarkdownSection(&content, "风险", task.Requirements.Risks)
	}
	if prompt := strings.TrimSpace(task.Requirements.ExecutionPrompt); prompt != "" {
		content.WriteByte('\n')
		content.WriteString("## 执行提示词\n\n")
		content.WriteString(prompt + "\n")
	}
	return content.String()
}

func renderAcceptanceCriteria(task TaskSnapshot) string {
	var content strings.Builder
	content.WriteString("# 验收标准\n\n")
	values := nonEmptyStrings(task.Requirements.AcceptanceCriteria)
	if len(values) == 0 {
		content.WriteString("- 尚未定义。\n")
	} else {
		for _, value := range values {
			content.WriteString("- [ ] " + value + "\n")
		}
	}
	return content.String()
}

func renderApprovedRequirement(revision ApprovedRequirementRevision) string {
	var content strings.Builder
	fmt.Fprintf(&content, "# 已确认需求 v%d\n\n", revision.Version)
	if confirmedAt := strings.TrimSpace(revision.ConfirmedAt); confirmedAt != "" {
		fmt.Fprintf(&content, "确认时间：`%s`\n\n", confirmedAt)
	}
	document := strings.TrimSpace(revision.Document)
	if document == "" {
		content.WriteString("暂无需求正文。\n")
	} else {
		content.WriteString(document + "\n")
	}
	if prompt := strings.TrimSpace(revision.ExecutionPrompt); prompt != "" {
		content.WriteString("\n## 执行提示词\n\n")
		content.WriteString(prompt + "\n")
	}
	return content.String()
}

func renderProjectObservation(observation ProjectObservation) string {
	var content strings.Builder
	content.WriteString("# 项目观察\n\n")
	if observation.FilePath != "" {
		fmt.Fprintf(&content, "- 文件: `%s`\n", observation.FilePath)
	}
	if observation.LineRange != "" {
		fmt.Fprintf(&content, "- 行号: `%s`\n", observation.LineRange)
	}
	content.WriteString("\n" + strings.TrimSpace(observation.Content) + "\n")
	return content.String()
}

func writeMarkdownSection(content *strings.Builder, title string, values []string) {
	content.WriteString("## " + title + "\n\n")
	filtered := nonEmptyStrings(values)
	if len(filtered) == 0 {
		content.WriteString("- 暂无。\n\n")
		return
	}
	for _, value := range filtered {
		content.WriteString("- " + value + "\n")
	}
	content.WriteByte('\n')
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func sourceDirectory(sourceType string) (string, string) {
	switch sourceType {
	case "manual":
		return "sources/manual", "manual"
	case "plane":
		return "sources/plane", "plane"
	case "chat":
		return "sources/chats", "chat"
	case "project":
		return "sources/project-observations", "project_observation"
	default:
		return "sources/manual", "manual"
	}
}

func appendEngineIdentityWarning(
	warnings []string,
	field string,
	value string,
) []string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "omp", "oh-my-pi", "pi / oh-my-pi":
		return append(warnings, field+" 的旧 PI 标识已按原生 pi 只读映射")
	case "", "pi", "codex":
		return warnings
	default:
		return append(warnings, field+" 含未知引擎标识，已保留原值并保持只读")
	}
}

func safeStem(name string, fallback string) string {
	name = strings.TrimSpace(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	var result strings.Builder
	lastSeparator := false
	for _, character := range name {
		allowed := unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_'
		if allowed {
			result.WriteRune(character)
			lastSeparator = false
		} else if !lastSeparator && result.Len() > 0 {
			result.WriteByte('-')
			lastSeparator = true
		}
		if result.Len() >= 60 {
			break
		}
	}
	value := strings.Trim(result.String(), "-_. ")
	if value == "" || windowsReservedName.MatchString(value) {
		value = strings.Trim(strings.Map(func(character rune) rune {
			if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_' {
				return character
			}
			return '-'
		}, fallback), "-_. ")
	}
	if value == "" || windowsReservedName.MatchString(value) {
		return "resource"
	}
	return value
}

func safeTextExtension(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".md", ".json", ".yaml", ".yml", ".csv":
		return strings.ToLower(filepath.Ext(name))
	default:
		return ".md"
	}
}

func safeExtension(name string, content []byte, image bool) string {
	if image {
		switch http.DetectContentType(content) {
		case "image/png":
			return ".png"
		case "image/jpeg":
			return ".jpg"
		case "image/gif":
			return ".gif"
		case "image/webp":
			return ".webp"
		}
	}
	return safeTextExtension(name)
}

func looksLikeImageFilename(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func mimeTypeFor(name string, content []byte) string {
	detected := http.DetectContentType(content)
	if strings.HasPrefix(detected, "image/") {
		return detected
	}
	if extension := filepath.Ext(name); extension != "" {
		if value := mime.TypeByExtension(extension); value != "" {
			value = strings.Split(value, ";")[0]
			if strings.HasPrefix(value, "image/") {
				return detected
			}
			return value
		}
	}
	return detected
}

type decodedImage struct {
	Content   []byte
	Extension string
}

func decodeDataImages(value string) ([]decodedImage, error) {
	matches := dataImagePattern.FindAllStringSubmatch(value, -1)
	if strings.Contains(value, "data:image/") && len(matches) == 0 {
		return nil, errors.New("malformed image data URL")
	}
	images := make([]decodedImage, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		mimeSubtype := strings.ToLower(match[1])
		extension := "." + mimeSubtype
		expectedMIME := "image/" + mimeSubtype
		if mimeSubtype == "jpeg" {
			extension = ".jpg"
			expectedMIME = "image/jpeg"
		} else if mimeSubtype == "jpg" {
			extension = ".jpg"
			expectedMIME = "image/jpeg"
		}
		switch extension {
		case ".png", ".jpg", ".gif", ".webp":
		default:
			return nil, fmt.Errorf("unsupported image data URL MIME %q", expectedMIME)
		}
		encoded := strings.NewReplacer("\r", "", "\n", "").Replace(match[2])
		content, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid image data URL: %w", err)
		}
		if len(content) == 0 {
			return nil, errors.New("image data URL is empty")
		}
		if len(content) > MaxAttachmentBytes {
			return nil, errors.New("image data URL exceeds the 16 MiB limit")
		}
		detectedMIME := http.DetectContentType(content)
		if detectedMIME != expectedMIME {
			return nil, fmt.Errorf(
				"image data URL MIME %q does not match bytes %q",
				expectedMIME,
				detectedMIME,
			)
		}
		hash := sha256.Sum256(content)
		key := hex.EncodeToString(hash[:])
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		images = append(images, decodedImage{Content: content, Extension: extension})
	}
	return images, nil
}

func walkLegacyFiles(
	root *os.Root,
	directory string,
	visit func(string, []byte) error,
) error {
	info, err := root.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%q is not a regular directory", directory)
	}
	return fs.WalkDir(root.FS(), directory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == directory {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("legacy path %q is a symbolic link", name)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("legacy path %q is not a regular file", name)
		}
		if info.Size() > MaxAttachmentBytes {
			return fmt.Errorf("legacy file %q exceeds the 16 MiB limit", name)
		}
		content, err := root.ReadFile(name)
		if err != nil {
			return err
		}
		return visit(name, content)
	})
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func isUTF8Text(content []byte) bool {
	return utf8.Valid(content) && !bytes.ContainsRune(content, '\x00')
}
