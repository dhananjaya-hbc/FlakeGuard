package mcp

import (
	"github.com/dhananjaya-hbc/FlakeGuard/internal/repository"
)

func RegisterTools(server *Server, repo *repository.Repository) {
	server.RegisterTool(checkTestFlakinessTool(), checkTestFlakinessHandler(repo))
	server.RegisterTool(getTestHistoryTool(), getTestHistoryHandler(repo))
	server.RegisterTool(listFlakiestTestsTool(), listFlakiestTestsHandler(repo))
}

func textResult(text string) *ToolCallResult {
	return &ToolCallResult{Content: []ContentBlock{{Type: "text", Text: text}}}
}

func flakinessVerdict(score float64) string {
	switch {
	case score == 0:
		return "stable — this test has never flipped between pass/fail"
	case score < 0.15:
		return "mostly stable — rare flips, a failure is probably a real bug"
	case score < 0.4:
		return "flaky — treat a failure with suspicion, verify before assuming it's a real bug"
	default:
		return "highly flaky — failures here are likely noise, not a real bug"
	}
}
