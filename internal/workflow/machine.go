package workflow

import "fmt"

type Status string

const (
	StatusInbox        Status = "inbox"
	StatusRequirements Status = "requirements"
	StatusApproved     Status = "approved"
	StatusDevelopment  Status = "development"
	StatusReview       Status = "review"
	StatusDone         Status = "done"
)

var orderedStatuses = []Status{
	StatusInbox,
	StatusRequirements,
	StatusApproved,
	StatusDevelopment,
	StatusReview,
	StatusDone,
}

type TransitionRequest struct {
	From                  Status
	To                    Status
	RequirementsConfirmed bool
	DevelopmentCompleted  bool
	ReviewApproved        bool
}

func indexOf(status Status) int {
	for index, candidate := range orderedStatuses {
		if candidate == status {
			return index
		}
	}
	return -1
}

// ValidateTransition permits only a single manual step at a time. Backward
// movement is always allowed; forward movement is guarded by explicit human
// confirmation and recorded work results.
func ValidateTransition(request TransitionRequest) error {
	fromIndex := indexOf(request.From)
	toIndex := indexOf(request.To)

	if fromIndex == -1 || toIndex == -1 {
		return fmt.Errorf("未知的任务状态：%s → %s", request.From, request.To)
	}
	if fromIndex == toIndex {
		return nil
	}
	if difference := toIndex - fromIndex; difference > 1 || difference < -1 {
		return fmt.Errorf("任务只能手动推进或退回一个阶段")
	}
	if toIndex < fromIndex {
		return nil
	}

	switch request.To {
	case StatusApproved:
		if !request.RequirementsConfirmed {
			return fmt.Errorf("需求尚未人工确认，不能进入待开发")
		}
	case StatusDevelopment:
		if !request.RequirementsConfirmed {
			return fmt.Errorf("需求确认已失效，不能开始开发")
		}
	case StatusReview:
		if !request.DevelopmentCompleted {
			return fmt.Errorf("尚未记录开发完成结果，不能进入审核")
		}
	case StatusDone:
		if !request.ReviewApproved {
			return fmt.Errorf("审核尚未人工通过，不能完成任务")
		}
	}

	return nil
}

