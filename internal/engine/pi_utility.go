package engine

import (
	"context"

	"github.com/blue7zz/BTaskAssistant/internal/agent"
)

var runNativePIUtility = func(
	ctx context.Context,
	request agent.UtilityRequest,
) (string, error) {
	return agent.RunUtility(ctx, request)
}
