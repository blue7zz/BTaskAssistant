package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/blue7zz/BTaskAssistant/internal/storage"
	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
)

const (
	maxPromptReferences  = 20
	maxPromptImages      = 5
	maxPromptImageBytes  = 4 * 1024 * 1024
	maxPromptImagesBytes = 10 * 1024 * 1024
)

type promptReferenceSet struct {
	references []storage.AgentReferenceRecord
	images     []map[string]any
	prompt     string
}

func (service *Service) Resources(request ResourceSearchRequest) ([]ResourceDescriptor, error) {
	if _, err := service.store.EnsureTaskWorkspace(request.TaskID); err != nil {
		return nil, err
	}
	request.Query = strings.ToLower(strings.TrimSpace(request.Query))
	if len(request.Query) > 200 || strings.ContainsRune(request.Query, '\x00') {
		return nil, errors.New("resource search query is invalid")
	}
	if request.Limit <= 0 {
		request.Limit = 50
	}
	if request.Limit > 100 {
		return nil, errors.New("resource search limit cannot exceed 100")
	}
	descriptors, err := service.allResourceDescriptors(request.TaskID)
	if err != nil {
		return nil, err
	}
	filtered := make([]ResourceDescriptor, 0, min(request.Limit, len(descriptors)))
	for _, descriptor := range descriptors {
		haystack := strings.ToLower(strings.Join([]string{
			descriptor.ID, descriptor.Kind, descriptor.SourceType, descriptor.LogicalPath,
		}, " "))
		if request.Query != "" && !strings.Contains(haystack, request.Query) {
			continue
		}
		filtered = append(filtered, descriptor)
		if len(filtered) == request.Limit {
			break
		}
	}
	return filtered, nil
}

func (service *Service) Artifacts(taskID string) ([]ResourceDescriptor, error) {
	if _, err := service.store.EnsureTaskWorkspace(taskID); err != nil {
		return nil, err
	}
	resources, err := service.allResourceDescriptors(taskID)
	if err != nil {
		return nil, err
	}
	artifacts := make([]ResourceDescriptor, 0)
	for _, resource := range resources {
		if resource.TargetType == "artifact" {
			artifacts = append(artifacts, resource)
		}
	}
	return artifacts, nil
}

func (service *Service) allResourceDescriptors(taskID string) ([]ResourceDescriptor, error) {
	resources, err := service.store.TaskResources(taskID)
	if err != nil {
		return nil, err
	}
	artifacts, err := service.store.WorkspaceArtifacts(taskID)
	if err != nil {
		return nil, err
	}
	proposals, err := service.store.RequirementProposals(taskID)
	if err != nil {
		return nil, err
	}
	proposalStates := make(map[string]string, len(proposals))
	for _, proposal := range proposals {
		proposalStates[proposal.ArtifactID] = proposal.State
	}
	result := make([]ResourceDescriptor, 0, len(resources)+len(artifacts))
	for _, resource := range resources {
		descriptor := ResourceDescriptor{
			ID: resource.ID, TaskID: resource.TaskID, TargetType: "resource",
			Kind: resource.Kind, SourceType: resource.SourceType,
			LogicalPath: resource.LogicalPath, Immutable: resource.Immutable,
			Readable: resource.Readable, CreatedAt: resource.CreatedAt,
		}
		if resource.MIMEType != nil {
			descriptor.MIMEType = *resource.MIMEType
		}
		if resource.ByteSize != nil {
			descriptor.ByteSize = *resource.ByteSize
		}
		if resource.SHA256 != nil {
			descriptor.SHA256 = *resource.SHA256
		}
		result = append(result, descriptor)
	}
	for _, artifact := range artifacts {
		descriptor := ResourceDescriptor{
			ID: artifact.ID, TaskID: artifact.TaskID, TargetType: "artifact",
			Kind: artifact.Kind, SourceType: "generated_artifact",
			LogicalPath: artifact.LogicalPath, ByteSize: artifact.ByteSize,
			SHA256: artifact.SHA256, Immutable: false, Readable: true,
			CreatedAt: artifact.CreatedAt, UpdatedAt: artifact.UpdatedAt,
		}
		if artifact.MIMEType != nil {
			descriptor.MIMEType = *artifact.MIMEType
		}
		if state, exists := proposalStates[artifact.ID]; exists {
			descriptor.ProposalState = &state
		}
		result = append(result, descriptor)
	}
	sort.Slice(result, func(left int, right int) bool {
		if result[left].TargetType == result[right].TargetType {
			return result[left].LogicalPath < result[right].LogicalPath
		}
		return result[left].TargetType == "resource"
	})
	return result, nil
}

func (service *Service) resourceDescriptor(taskID string, resourceID string) (ResourceDescriptor, error) {
	if strings.TrimSpace(resourceID) == "" || len(resourceID) > 200 {
		return ResourceDescriptor{}, errors.New("resource id is invalid")
	}
	resource, err := service.store.TaskResource(taskID, resourceID)
	if err == nil {
		result := ResourceDescriptor{
			ID: resource.ID, TaskID: resource.TaskID, TargetType: "resource",
			Kind: resource.Kind, SourceType: resource.SourceType,
			LogicalPath: resource.LogicalPath, Immutable: resource.Immutable,
			Readable: resource.Readable, CreatedAt: resource.CreatedAt,
		}
		if resource.MIMEType != nil {
			result.MIMEType = *resource.MIMEType
		}
		if resource.ByteSize != nil {
			result.ByteSize = *resource.ByteSize
		}
		if resource.SHA256 != nil {
			result.SHA256 = *resource.SHA256
		}
		return result, nil
	}
	artifact, artifactErr := service.store.WorkspaceArtifact(taskID, resourceID)
	if artifactErr != nil {
		return ResourceDescriptor{}, err
	}
	result := ResourceDescriptor{
		ID: artifact.ID, TaskID: artifact.TaskID, TargetType: "artifact",
		Kind: artifact.Kind, SourceType: "generated_artifact",
		LogicalPath: artifact.LogicalPath, ByteSize: artifact.ByteSize,
		SHA256: artifact.SHA256, Readable: true, CreatedAt: artifact.CreatedAt,
		UpdatedAt: artifact.UpdatedAt,
	}
	if artifact.MIMEType != nil {
		result.MIMEType = *artifact.MIMEType
	}
	return result, nil
}

func (service *Service) ImportAttachments(
	request ImportAttachmentsRequest,
) ([]ResourceDescriptor, error) {
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return nil, err
	}
	if len(request.Files) == 0 || len(request.Files) > taskspace.MaxAttachmentCount {
		return nil, fmt.Errorf("attachment count must be between 1 and %d", taskspace.MaxAttachmentCount)
	}
	inputs := make([]taskspace.AttachmentInput, 0, len(request.Files))
	for _, file := range request.Files {
		if len(file.DataBase64) > ((taskspace.MaxAttachmentBytes+2)/3)*4+4 {
			return nil, fmt.Errorf("attachment %q exceeds the encoded size limit", file.Name)
		}
		content, err := base64.StdEncoding.DecodeString(file.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("attachment %q is not valid base64", file.Name)
		}
		inputs = append(inputs, taskspace.AttachmentInput{
			Name: file.Name, MIMEType: file.MIMEType, Content: content,
		})
	}
	imported, err := (taskspace.Service{}).ImportAttachments(
		filepath.Dir(workspace.RootPath), request.TaskID, inputs,
	)
	if err != nil {
		return nil, err
	}
	if _, err := service.store.EnsureTaskWorkspace(request.TaskID); err != nil {
		return nil, err
	}
	result := make([]ResourceDescriptor, 0, len(imported))
	for _, resource := range imported {
		descriptor, err := service.resourceDescriptor(request.TaskID, resource.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, descriptor)
	}
	return result, nil
}

func (service *Service) PreviewResource(
	request ResourcePreviewRequest,
) (taskspace.FilePreview, error) {
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return taskspace.FilePreview{}, err
	}
	resource, err := service.resourceDescriptor(request.TaskID, request.ResourceID)
	if err != nil {
		return taskspace.FilePreview{}, err
	}
	return (taskspace.Service{}).Read(
		filepath.Dir(workspace.RootPath), request.TaskID, resource.LogicalPath,
	)
}

func (service *Service) RemoveReference(request RemoveReferenceRequest) error {
	if _, err := service.store.AgentSession(request.TaskID, request.SessionID); err != nil {
		return err
	}
	return service.store.RemoveAgentMessageReference(
		request.TaskID, request.SessionID, request.MessageID, request.ResourceID,
	)
}

func (service *Service) resolvePromptReferences(
	workspace storage.TaskWorkspaceRecord,
	request PromptRequest,
	createdAt string,
) (promptReferenceSet, error) {
	if len(request.ResourceIDs) > maxPromptReferences {
		return promptReferenceSet{}, fmt.Errorf("a message may reference at most %d resources", maxPromptReferences)
	}
	result := promptReferenceSet{prompt: request.Message}
	seen := make(map[string]struct{}, len(request.ResourceIDs))
	var imageBytes int64
	manifest := make([]string, 0, len(request.ResourceIDs))
	for _, resourceID := range request.ResourceIDs {
		if _, exists := seen[resourceID]; exists {
			continue
		}
		seen[resourceID] = struct{}{}
		descriptor, err := service.resourceDescriptor(request.TaskID, resourceID)
		if err != nil {
			return promptReferenceSet{}, fmt.Errorf("current task resource %q is unavailable: %w", resourceID, err)
		}
		method := "mention"
		if descriptor.TargetType == "resource" && descriptor.Kind == "attachment" {
			method = "attachment"
		}
		result.references = append(result.references, storage.AgentReferenceRecord{
			ResourceID: descriptor.ID, TargetType: descriptor.TargetType,
			Method: method, CreatedAt: createdAt,
		})
		manifest = append(manifest, fmt.Sprintf(
			"- id `%s`; kind `%s`; path `%s`", descriptor.ID, descriptor.Kind, descriptor.LogicalPath,
		))
		if !strings.HasPrefix(descriptor.MIMEType, "image/") {
			continue
		}
		if len(result.images) >= maxPromptImages {
			return promptReferenceSet{}, fmt.Errorf("a prompt may include at most %d images", maxPromptImages)
		}
		content, mimeType, err := (taskspace.Service{}).ReadBytes(
			filepath.Dir(workspace.RootPath), request.TaskID, descriptor.LogicalPath,
			maxPromptImageBytes,
		)
		if err != nil {
			return promptReferenceSet{}, fmt.Errorf("read referenced image: %w", err)
		}
		if mimeType != descriptor.MIMEType || !supportedPromptImageMIME(mimeType) {
			return promptReferenceSet{}, errors.New("referenced image MIME no longer matches its resource index")
		}
		imageBytes += int64(len(content))
		if imageBytes > maxPromptImagesBytes {
			return promptReferenceSet{}, errors.New("referenced images exceed the 10 MiB prompt limit")
		}
		result.images = append(result.images, map[string]any{
			"type": "image", "data": base64.StdEncoding.EncodeToString(content), "mimeType": mimeType,
		})
	}
	if len(manifest) > 0 {
		result.prompt += "\n\n[BTask current-task references; use btask_read_resource for document contents]\n" +
			strings.Join(manifest, "\n")
	}
	return result, nil
}

func supportedPromptImageMIME(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func (service *Service) gateResourceContent(
	taskID string,
	resourceID string,
) (map[string]any, error) {
	preview, err := service.PreviewResource(ResourcePreviewRequest{
		TaskID: taskID, ResourceID: resourceID,
	})
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"resourceId": resourceID,
		"path":       preview.Path,
		"mimeType":   preview.MIMEType,
		"byteSize":   preview.ByteSize,
		"sha256":     preview.SHA256,
		"kind":       preview.Kind,
	}
	if preview.Kind == "image" {
		prefix := "data:" + preview.MIMEType + ";base64,"
		if !strings.HasPrefix(preview.Content, prefix) {
			return nil, errors.New("image preview encoding is invalid")
		}
		result["image"] = map[string]any{
			"data":     strings.TrimPrefix(preview.Content, prefix),
			"mimeType": preview.MIMEType,
		}
	} else {
		if !utf8.ValidString(preview.Content) {
			return nil, errors.New("resource text is not valid UTF-8")
		}
		result["text"] = preview.Content
	}
	return result, nil
}

func (service *Service) writeArtifactLocked(
	managed *managedSession,
	kind string,
	name string,
	content string,
) (ResourceDescriptor, error) {
	if managed.run == nil {
		return ResourceDescriptor{}, errors.New("artifact write requires an active PI run")
	}
	if managed.record.Mode == "ask" {
		return ResourceDescriptor{}, errors.New("Ask mode cannot write artifacts")
	}
	workspace, err := service.store.EnsureTaskWorkspace(managed.record.TaskID)
	if err != nil {
		return ResourceDescriptor{}, err
	}
	written, err := (taskspace.Service{}).WriteArtifact(
		filepath.Dir(workspace.RootPath), managed.record.TaskID, kind, name, []byte(content),
	)
	if err != nil {
		return ResourceDescriptor{}, err
	}
	if _, err := service.store.EnsureTaskWorkspace(managed.record.TaskID); err != nil {
		return ResourceDescriptor{}, err
	}
	now := service.timestamp()
	artifactID := artifactID(managed.record.TaskID, written.LogicalPath)
	createdAt := now
	if existing, err := service.store.WorkspaceArtifactByPath(managed.record.TaskID, written.LogicalPath); err == nil {
		artifactID = existing.ID
		createdAt = existing.CreatedAt
	} else if !errors.Is(err, storage.ErrAgentDataNotFound) {
		return ResourceDescriptor{}, err
	}
	sessionID := managed.record.ID
	runID := managed.run.record.ID
	mimeType := written.MIMEType
	record := storage.WorkspaceArtifactRecord{
		ID: artifactID, TaskID: managed.record.TaskID, SessionID: &sessionID,
		RunID: &runID, LogicalPath: written.LogicalPath, Kind: written.Kind,
		MIMEType: &mimeType, ByteSize: written.ByteSize, SHA256: written.SHA256,
		CreatedAt: createdAt, UpdatedAt: now,
	}
	if err := service.store.UpsertWorkspaceArtifact(record); err != nil {
		return ResourceDescriptor{}, err
	}
	if written.Kind == "proposal" {
		revision, err := service.store.TaskRevision(managed.record.TaskID)
		if err != nil {
			return ResourceDescriptor{}, err
		}
		if err := service.store.UpsertRequirementProposal(storage.RequirementProposalRecord{
			TaskID: managed.record.TaskID, ArtifactID: artifactID,
			BaseRevision: revision, State: "pending", CreatedAt: createdAt,
		}); err != nil {
			return ResourceDescriptor{}, err
		}
	}
	references, err := service.store.AgentMessageReferences(
		managed.record.TaskID, managed.record.ID, managed.run.assistant.ID,
	)
	if err != nil {
		return ResourceDescriptor{}, err
	}
	position := 0
	alreadyLinked := false
	for _, reference := range references {
		if reference.Position >= position {
			position = reference.Position + 1
		}
		if reference.ResourceID == artifactID && reference.Method == "generated" {
			alreadyLinked = true
		}
	}
	if !alreadyLinked {
		if err := service.store.AddAgentMessageReference(storage.AgentReferenceRecord{
			TaskID: managed.record.TaskID, SessionID: managed.record.ID,
			MessageID: managed.run.assistant.ID, ResourceID: artifactID,
			TargetType: "artifact", Method: "generated", Position: position,
			CreatedAt: now,
		}); err != nil {
			return ResourceDescriptor{}, err
		}
	}
	descriptor, err := service.resourceDescriptor(managed.record.TaskID, artifactID)
	if err != nil {
		return ResourceDescriptor{}, err
	}
	_ = service.emitLocked(managed, "workspace.changed", managed.run.record.ID, "", map[string]any{
		"reason": "artifact", "resourceId": descriptor.ID,
		"paths": []string{descriptor.LogicalPath},
	})
	return descriptor, nil
}

func artifactID(taskID string, logicalPath string) string {
	hash := sha256.Sum256([]byte(taskID + "\x00" + logicalPath))
	return "artifact_" + hex.EncodeToString(hash[:16])
}

func (service *Service) gateListResources(
	taskID string,
	query string,
	limit int,
) ([]ResourceDescriptor, error) {
	return service.Resources(ResourceSearchRequest{TaskID: taskID, Query: query, Limit: limit})
}

func sendRuntimeNotification(ctx context.Context, runtime Runtime, fields map[string]any) error {
	sender, ok := runtime.(interface {
		Send(context.Context, map[string]any) error
	})
	if !ok {
		return errors.New("PI runtime does not support extension bridge responses")
	}
	return sender.Send(ctx, fields)
}
