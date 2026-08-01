package gitrepo

import "github.com/blue7zz/BTaskAssistant/internal/storage"

const (
	StateUnbound = "unbound"
	maxDiffBytes = 2 * 1024 * 1024
)

type BindRequest struct {
	TaskID     string `json:"taskId"`
	SourcePath string `json:"sourcePath"`
}

type StatusView struct {
	Bound           bool                     `json:"bound"`
	Binding         storage.GitBindingRecord `json:"binding"`
	RemoteURL       string                   `json:"remoteUrl,omitempty"`
	Head            string                   `json:"head,omitempty"`
	Snapshot        string                   `json:"snapshot,omitempty"`
	Files           []ChangedFile            `json:"files"`
	AheadOfBaseline int                      `json:"aheadOfBaseline"`
	BehindBaseline  int                      `json:"behindBaseline"`
	ErrorMessage    string                   `json:"errorMessage,omitempty"`
}

type ChangedFile struct {
	Path           string `json:"path"`
	OriginalPath   string `json:"originalPath,omitempty"`
	Status         string `json:"status"`
	IndexStatus    string `json:"indexStatus"`
	WorktreeStatus string `json:"worktreeStatus"`
	Staged         bool   `json:"staged"`
	Unstaged       bool   `json:"unstaged"`
}

type FileDiffView struct {
	TaskID    string `json:"taskId"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	Staged    string `json:"staged"`
	Unstaged  string `json:"unstaged"`
	Added     int    `json:"added"`
	Removed   int    `json:"removed"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	ByteSize  int64  `json:"byteSize"`
}

type FileEntry struct {
	Path       string `json:"path"`
	Tracked    bool   `json:"tracked"`
	ByteSize   int64  `json:"byteSize"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

type FileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	ByteSize int64  `json:"byteSize"`
	SHA256   string `json:"sha256"`
}

type FileWriteResult struct {
	Path        string `json:"path"`
	Operation   string `json:"operation"`
	PreviousSHA string `json:"previousSha256,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	ByteSize    int64  `json:"byteSize"`
}

type CommitRequest struct {
	TaskID           string `json:"taskId"`
	Message          string `json:"message"`
	ExpectedSnapshot string `json:"expectedSnapshot"`
	Confirmed        bool   `json:"confirmed"`
}

type CommitResult struct {
	Commit string     `json:"commit"`
	Status StatusView `json:"status"`
}

type CleanupRequest struct {
	TaskID    string `json:"taskId"`
	Confirmed bool   `json:"confirmed"`
}

type RecoverRequest struct {
	TaskID    string `json:"taskId"`
	Confirmed bool   `json:"confirmed"`
}
