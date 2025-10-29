package http

import (
	"fmt"
	"strings"
	"time"

	"github.com/imalsogreg/beads/internal/rpc"
	"github.com/imalsogreg/beads/internal/types"
)

// formatIssue formats a single issue for create operations
func (s *Server) formatIssue(issue *types.Issue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✓ Created issue: %s\n", issue.ID)
	fmt.Fprintf(&b, "  Title: %s\n", issue.Title)
	if issue.Priority >= 0 && issue.Priority <= 4 {
		fmt.Fprintf(&b, "  Priority: P%d\n", issue.Priority)
	}
	fmt.Fprintf(&b, "  Status: %s\n", issue.Status)
	if issue.Assignee != "" {
		fmt.Fprintf(&b, "  Assignee: %s\n", issue.Assignee)
	}
	return b.String()
}

// formatIssueList formats a list of issues
func (s *Server) formatIssueList(issues []*types.Issue) string {
	if len(issues) == 0 {
		return "No issues found.\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\nFound %d issue(s):\n\n", len(issues))

	for _, issue := range issues {
		priority := ""
		if issue.Priority >= 0 && issue.Priority <= 4 {
			priority = fmt.Sprintf(" [P%d]", issue.Priority)
		}
		issueType := ""
		if issue.IssueType != "" {
			issueType = fmt.Sprintf(" [%s]", issue.IssueType)
		}
		assignee := ""
		if issue.Assignee != "" {
			assignee = fmt.Sprintf(" (@%s)", issue.Assignee)
		}

		fmt.Fprintf(&b, "%s%s%s %s%s\n", issue.ID, priority, issueType, issue.Status, assignee)
		fmt.Fprintf(&b, "  %s\n\n", issue.Title)
	}

	return b.String()
}

// formatIssueDetail formats detailed issue information
func (s *Server) formatIssueDetail(issue *types.Issue) string {
	var b strings.Builder

	fmt.Fprintf(&b, "\n%s: %s\n", issue.ID, issue.Title)
	fmt.Fprintf(&b, strings.Repeat("=", len(issue.ID)+len(issue.Title)+2)+"\n\n")

	fmt.Fprintf(&b, "Status: %s\n", issue.Status)
	if issue.Priority >= 0 && issue.Priority <= 4 {
		fmt.Fprintf(&b, "Priority: P%d\n", issue.Priority)
	}
	fmt.Fprintf(&b, "Type: %s\n", issue.IssueType)

	if issue.Assignee != "" {
		fmt.Fprintf(&b, "Assignee: %s\n", issue.Assignee)
	}

	if issue.EstimatedMinutes != nil && *issue.EstimatedMinutes > 0 {
		hours := *issue.EstimatedMinutes / 60
		minutes := *issue.EstimatedMinutes % 60
		if hours > 0 {
			fmt.Fprintf(&b, "Estimated: %dh %dm\n", hours, minutes)
		} else {
			fmt.Fprintf(&b, "Estimated: %dm\n", minutes)
		}
	}

	fmt.Fprintf(&b, "\nCreated: %s\n", issue.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "Updated: %s\n", issue.UpdatedAt.Format("2006-01-02 15:04:05"))

	if issue.ClosedAt != nil {
		fmt.Fprintf(&b, "Closed: %s\n", issue.ClosedAt.Format("2006-01-02 15:04:05"))
	}

	if issue.Description != "" {
		fmt.Fprintf(&b, "\nDescription:\n%s\n", issue.Description)
	}

	if issue.Design != "" {
		fmt.Fprintf(&b, "\nDesign:\n%s\n", issue.Design)
	}

	if issue.AcceptanceCriteria != "" {
		fmt.Fprintf(&b, "\nAcceptance Criteria:\n%s\n", issue.AcceptanceCriteria)
	}

	if issue.Notes != "" {
		fmt.Fprintf(&b, "\nNotes:\n%s\n", issue.Notes)
	}

	if len(issue.Labels) > 0 {
		fmt.Fprintf(&b, "\nLabels: %s\n", strings.Join(issue.Labels, ", "))
	}

	if len(issue.Dependencies) > 0 {
		fmt.Fprintf(&b, "\nDependencies:\n")
		for _, dep := range issue.Dependencies {
			fmt.Fprintf(&b, "  - %s %s (type: %s)\n", issue.ID, dep.DependsOnID, dep.Type)
		}
	}

	if len(issue.Comments) > 0 {
		fmt.Fprintf(&b, "\nComments (%d):\n", len(issue.Comments))
		for _, comment := range issue.Comments {
			fmt.Fprintf(&b, "  [%s] %s: %s\n",
				comment.CreatedAt.Format("2006-01-02 15:04"),
				comment.Author,
				comment.Text)
		}
	}

	return b.String()
}

// formatReadyWork formats ready work list
func (s *Server) formatReadyWork(issues []*types.Issue) string {
	if len(issues) == 0 {
		return "\nNo ready work found.\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n📋 Ready work (%d issue(s) with no blockers):\n\n", len(issues))

	for i, issue := range issues {
		priority := ""
		if issue.Priority >= 0 && issue.Priority <= 4 {
			priority = fmt.Sprintf(" [P%d]", issue.Priority)
		}
		assignee := ""
		if issue.Assignee != "" {
			assignee = fmt.Sprintf(" (@%s)", issue.Assignee)
		}

		fmt.Fprintf(&b, "%d. %s%s: %s%s\n", i+1, issue.ID, priority, issue.Title, assignee)
	}

	return b.String()
}

// formatStats formats database statistics
func (s *Server) formatStats(stats *types.Statistics) string {
	var b strings.Builder

	fmt.Fprintf(&b, "\n📊 Database Statistics\n")
	fmt.Fprintf(&b, "=====================\n\n")

	fmt.Fprintf(&b, "Total Issues: %d\n", stats.TotalIssues)
	fmt.Fprintf(&b, "Open: %d\n", stats.OpenIssues)
	fmt.Fprintf(&b, "In Progress: %d\n", stats.InProgressIssues)
	fmt.Fprintf(&b, "Closed: %d\n", stats.ClosedIssues)
	fmt.Fprintf(&b, "Blocked: %d\n", stats.BlockedIssues)
	fmt.Fprintf(&b, "Ready: %d\n", stats.ReadyIssues)
	fmt.Fprintf(&b, "Epics Eligible for Closure: %d\n", stats.EpicsEligibleForClosure)
	if stats.AverageLeadTime > 0 {
		fmt.Fprintf(&b, "Average Lead Time: %.1f hours\n", stats.AverageLeadTime)
	}

	return b.String()
}

// formatEpicStatus formats epic status (expects array of EpicStatus)
func (s *Server) formatEpicStatus(statuses []*types.EpicStatus) string {
	if len(statuses) == 0 {
		return "\nNo epics found.\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n🎯 Epic Status\n")
	fmt.Fprintf(&b, "==============\n\n")

	for _, status := range statuses {
		epicID := status.Epic.ID
		epicTitle := status.Epic.Title

		fmt.Fprintf(&b, "%s: %s\n", epicID, epicTitle)

		total := status.TotalChildren
		completed := status.ClosedChildren
		percentage := 0.0
		if total > 0 {
			percentage = float64(completed) / float64(total) * 100.0
		}

		fmt.Fprintf(&b, "Progress: %d/%d (%.1f%%)", completed, total, percentage)

		if status.EligibleForClose {
			fmt.Fprintf(&b, " ✅ Eligible for closure")
		}
		fmt.Fprintf(&b, "\n")

		if total > 0 {
			// Simple progress bar
			barWidth := 40
			filled := int(float64(barWidth) * percentage / 100.0)
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
			fmt.Fprintf(&b, "[%s]\n", bar)
		}

		fmt.Fprintf(&b, "\n")
	}

	return b.String()
}

// formatDependencyTree formats dependency tree
func (s *Server) formatDependencyTree(tree []*types.TreeNode) string {
	if len(tree) == 0 {
		return "\nNo dependencies found.\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n🌲 Dependency tree:\n\n")

	for _, node := range tree {
		indent := strings.Repeat("  ", node.Depth)
		priority := ""
		if node.Priority >= 0 && node.Priority <= 4 {
			priority = fmt.Sprintf(" [P%d]", node.Priority)
		}

		truncated := ""
		if node.Truncated {
			truncated = " [truncated]"
		}

		fmt.Fprintf(&b, "%s→ %s: %s%s (%s)%s\n", indent, node.ID, node.Title, priority, node.Status, truncated)
	}

	return b.String()
}

// formatComments formats comment list
func (s *Server) formatComments(comments []*types.Comment) string {
	if len(comments) == 0 {
		return "\nNo comments.\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n💬 Comments (%d):\n\n", len(comments))

	for i, comment := range comments {
		fmt.Fprintf(&b, "%d. [%s] %s:\n", i+1,
			comment.CreatedAt.Format("2006-01-02 15:04"),
			comment.Author)
		fmt.Fprintf(&b, "   %s\n\n", comment.Text)
	}

	return b.String()
}

// formatHealth formats health check result
func (s *Server) formatHealth(health *rpc.HealthResponse) string {
	var b strings.Builder

	status := "✓"
	if health.Status != "healthy" {
		status = "✗"
	}

	fmt.Fprintf(&b, "\n%s Health Check\n", status)
	fmt.Fprintf(&b, "==============\n\n")
	fmt.Fprintf(&b, "Status: %s\n", health.Status)
	fmt.Fprintf(&b, "Version: %s\n", health.Version)
	fmt.Fprintf(&b, "Compatible: %v\n", health.Compatible)
	fmt.Fprintf(&b, "Database Response Time: %.2fms\n", health.DBResponseTime)
	fmt.Fprintf(&b, "Uptime: %v\n", time.Duration(health.Uptime)*time.Second)
	fmt.Fprintf(&b, "Active Connections: %d/%d\n", health.ActiveConns, health.MaxConns)
	fmt.Fprintf(&b, "Memory: %d MB\n", health.MemoryAllocMB)

	if health.Error != "" {
		fmt.Fprintf(&b, "\nError: %s\n", health.Error)
	}

	return b.String()
}

// formatStatus formats status result
func (s *Server) formatStatus(status *rpc.StatusResponse) string {
	var b strings.Builder

	fmt.Fprintf(&b, "\n📡 Server Status\n")
	fmt.Fprintf(&b, "===============\n\n")
	fmt.Fprintf(&b, "Version: %s\n", status.Version)
	fmt.Fprintf(&b, "PID: %d\n", status.PID)
	fmt.Fprintf(&b, "Uptime: %v\n", time.Duration(status.UptimeSeconds)*time.Second)
	fmt.Fprintf(&b, "Workspace: %s\n", status.WorkspacePath)
	fmt.Fprintf(&b, "Database: %s\n", status.DatabasePath)

	if status.SocketPath != "" {
		fmt.Fprintf(&b, "Socket: %s\n", status.SocketPath)
	}

	if status.ExclusiveLockActive {
		fmt.Fprintf(&b, "Exclusive Lock: active (holder: %s)\n", status.ExclusiveLockHolder)
	}

	return b.String()
}

// formatMetrics formats metrics result
func (s *Server) formatMetrics(metrics *rpc.MetricsSnapshot) string {
	var b strings.Builder

	fmt.Fprintf(&b, "\n📈 Server Metrics\n")
	fmt.Fprintf(&b, "================\n\n")
	fmt.Fprintf(&b, "Uptime: %v\n", time.Duration(metrics.UptimeSeconds)*time.Second)
	fmt.Fprintf(&b, "Total Connections: %d\n", metrics.TotalConns)
	fmt.Fprintf(&b, "Active Connections: %d\n", metrics.ActiveConns)
	fmt.Fprintf(&b, "Rejected Connections: %d\n", metrics.RejectedConns)
	fmt.Fprintf(&b, "Memory: %d MB (sys: %d MB)\n", metrics.MemoryAllocMB, metrics.MemorySysMB)
	fmt.Fprintf(&b, "Goroutines: %d\n", metrics.GoroutineCount)

	if len(metrics.Operations) > 0 {
		fmt.Fprintf(&b, "\nOperations:\n")
		for _, op := range metrics.Operations {
			fmt.Fprintf(&b, "  %s: %d total, %d success, %d errors\n",
				op.Operation, op.TotalCount, op.SuccessCount, op.ErrorCount)
			if op.Latency.P50MS > 0 {
				fmt.Fprintf(&b, "    Latency: p50=%.2fms, p95=%.2fms, p99=%.2fms\n",
					op.Latency.P50MS, op.Latency.P95MS, op.Latency.P99MS)
			}
		}
	}

	return b.String()
}

// formatCompactStats formats compaction statistics
func (s *Server) formatCompactStats(stats *rpc.CompactStatsData) string {
	var b strings.Builder

	fmt.Fprintf(&b, "\n🗜️  Compaction Statistics\n")
	fmt.Fprintf(&b, "======================\n\n")
	fmt.Fprintf(&b, "Tier 1 Candidates: %d\n", stats.Tier1Candidates)
	fmt.Fprintf(&b, "Tier 2 Candidates: %d\n", stats.Tier2Candidates)
	fmt.Fprintf(&b, "Total Closed: %d\n", stats.TotalClosed)
	fmt.Fprintf(&b, "Tier 1 Min Age: %s\n", stats.Tier1MinAge)
	fmt.Fprintf(&b, "Tier 2 Min Age: %s\n", stats.Tier2MinAge)

	if stats.EstimatedSavings != "" {
		fmt.Fprintf(&b, "Estimated Savings: %s\n", stats.EstimatedSavings)
	}

	return b.String()
}
