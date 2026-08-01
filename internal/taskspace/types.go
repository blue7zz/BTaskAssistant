package taskspace

import "encoding/json"

const (
	SchemaVersion           = 1
	ResourcePolicy          = "isolated"
	ManagedBy               = "BTaskAssistant"
	MaxAttachmentBytes      = 16 * 1024 * 1024
	MaxAttachmentCount      = 10
	MaxAttachmentBatchBytes = 32 * 1024 * 1024
	MaxArtifactBytes        = 2 * 1024 * 1024
)

type Evidence struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

type ApprovedRequirementRevision struct {
	Version         int    `json:"version"`
	Document        string `json:"document"`
	ExecutionPrompt string `json:"executionPrompt"`
	ConfirmedAt     string `json:"confirmedAt"`
}

type ProjectObservation struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	FilePath  string `json:"filePath"`
	LineRange string `json:"lineRange"`
}

type Requirements struct {
	Objective          string                        `json:"objective"`
	Scope              []string                      `json:"scope"`
	OutOfScope         []string                      `json:"outOfScope"`
	AcceptanceCriteria []string                      `json:"acceptanceCriteria"`
	Risks              []string                      `json:"risks"`
	Document           string                        `json:"document"`
	ExecutionPrompt    string                        `json:"executionPrompt"`
	ApprovedRevisions  []ApprovedRequirementRevision `json:"approvedRevisions"`
	Interview          struct {
		Analyst             string               `json:"analyst"`
		ProjectObservations []ProjectObservation `json:"projectObservations"`
	} `json:"interview"`
}

type TaskSnapshot struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Summary      string       `json:"summary"`
	ProjectName  string       `json:"projectName"`
	ProjectPath  string       `json:"projectPath"`
	Priority     string       `json:"priority"`
	Status       string       `json:"status"`
	Revision     int          `json:"revision"`
	CreatedAt    string       `json:"createdAt"`
	UpdatedAt    string       `json:"updatedAt"`
	Evidence     []Evidence   `json:"evidence"`
	Requirements Requirements `json:"requirements"`
	Development  struct {
		Engine string `json:"engine"`
	} `json:"development"`
	Archived bool            `json:"-"`
	RawJSON  json.RawMessage `json:"-"`
}

type Manifest struct {
	SchemaVersion    int    `json:"schemaVersion"`
	TaskID           string `json:"taskId"`
	WorkspaceID      string `json:"workspaceId"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
	Revision         int    `json:"revision"`
	Engine           string `json:"engine"`
	ResourcePolicy   string `json:"resourcePolicy"`
	PIResourcePolicy string `json:"piResourcePolicy"`
	ManagedBy        string `json:"managedBy"`
}

type Resource struct {
	ID          string `json:"id"`
	TaskID      string `json:"taskId"`
	Kind        string `json:"kind"`
	SourceType  string `json:"sourceType"`
	LogicalPath string `json:"logicalPath"`
	StoragePath string `json:"storagePath,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
	ByteSize    int64  `json:"byteSize"`
	SHA256      string `json:"sha256"`
	Immutable   bool   `json:"immutable"`
	Readable    bool   `json:"readable"`
	CreatedAt   string `json:"createdAt"`
}

type Result struct {
	TaskID               string
	WorkspaceID          string
	RootPath             string
	SchemaVersion        int
	ManifestRevision     int
	CreatedAt            string
	UpdatedAt            string
	LegacyContextPath    string
	MigrationStartedAt   string
	MigrationCompletedAt string
	Warnings             []string
	Resources            []Resource
}

type WorkspaceEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	ByteSize   int64  `json:"byteSize"`
	ModifiedAt string `json:"modifiedAt"`
	Readable   bool   `json:"readable"`
}

type FilePreview struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	MIMEType  string `json:"mimeType"`
	ByteSize  int64  `json:"byteSize"`
	SHA256    string `json:"sha256"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type AttachmentInput struct {
	Name     string
	MIMEType string
	Content  []byte
}

type ArtifactFile struct {
	LogicalPath string `json:"logicalPath"`
	Kind        string `json:"kind"`
	MIMEType    string `json:"mimeType"`
	ByteSize    int64  `json:"byteSize"`
	SHA256      string `json:"sha256"`
}

type resourcesMirror struct {
	SchemaVersion int        `json:"schemaVersion"`
	TaskID        string     `json:"taskId"`
	Resources     []Resource `json:"resources"`
}
