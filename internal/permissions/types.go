package permissions

import "time"

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type Scope string

const (
	ScopeOnce      Scope = "once"
	ScopeSession   Scope = "session"
	ScopeTask      Scope = "task"
	ScopePermanent Scope = "permanent"
)

type Outcome string

const (
	OutcomeAllow Outcome = "allow"
	OutcomeDeny  Outcome = "deny"
	OutcomeAsk   Outcome = "ask"
)

type Classification struct {
	Capability       string
	Subject          string
	Target           string
	NormalizedTarget string
	ArgsDigest       string
	RiskLevel        RiskLevel
	ReadOnly         bool
	Mutating         bool
	HardDenyReason   string
}

type Request struct {
	RequestID      string
	TaskID         string
	SessionID      string
	RunID          string
	ToolCallID     string
	ToolName       string
	Mode           string
	TaskStatus     string
	GateValid      bool
	Classification Classification
}

type Grant struct {
	ID            string
	TaskID        string
	SessionID     string
	RequestID     string
	Capability    string
	TargetPattern string
	Scope         Scope
	Decision      Outcome
	RiskCeiling   RiskLevel
	ExpiresAt     *time.Time
	ConsumedAt    *time.Time
	RevokedAt     *time.Time
}

type Decision struct {
	Outcome        Outcome
	RiskLevel      RiskLevel
	Reason         string
	AllowedScopes  []Scope
	MatchedGrantID string
	ConsumeGrantID string
	HardDeny       bool
}

type PathTarget struct {
	RootKind     string `json:"rootKind"`
	RootID       string `json:"rootId"`
	RelativePath string `json:"relativePath"`
	Operation    string `json:"operation"`
}
