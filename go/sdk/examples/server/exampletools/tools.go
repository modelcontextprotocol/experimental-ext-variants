// Package exampletools defines the shared tool types and handlers used by the
// variant-aware MCP server examples. The tools simulate a developer
// productivity platform (à la GitHub/GitLab) with four variants:
//
//   - code-review: PR operations, diffs, and review comments
//   - project-management: issue tracking, labels, and assignments
//   - security-readonly: security scanning alerts and advisories (read-only)
//   - ci-automation: CI/CD workflow management and dispatch
package exampletools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// -- Code Review variant tools ------------------------------------------------

type ListPullRequestsInput struct {
	Repo   string `json:"repo" jsonschema:"repository in owner/name format"`
	State  string `json:"state,omitempty" jsonschema:"filter by state: open, closed, all (default: open)"`
	Author string `json:"author,omitempty" jsonschema:"filter by PR author username"`
}

type PullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author string `json:"author"`
	State  string `json:"state"`
}

type ListPullRequestsOutput struct {
	PullRequests []PullRequest `json:"pullRequests"`
}

func ListPullRequests(_ context.Context, _ *mcp.CallToolRequest, in ListPullRequestsInput) (*mcp.CallToolResult, ListPullRequestsOutput, error) {
	return nil, ListPullRequestsOutput{
		PullRequests: []PullRequest{
			{Number: 42, Title: "Add retry logic to API client", Author: "alice", State: "open"},
			{Number: 38, Title: "Fix race condition in cache layer", Author: "bob", State: "open"},
			{Number: 35, Title: "Update dependencies to latest", Author: "alice", State: "open"},
		},
	}, nil
}

type GetDiffInput struct {
	Repo   string `json:"repo" jsonschema:"repository in owner/name format"`
	Number int    `json:"number" jsonschema:"pull request number"`
}

type GetDiffOutput struct {
	Diff      string   `json:"diff"`
	Files     []string `json:"filesChanged"`
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
}

func GetDiff(_ context.Context, _ *mcp.CallToolRequest, in GetDiffInput) (*mcp.CallToolResult, GetDiffOutput, error) {
	return nil, GetDiffOutput{
		Diff:      "--- a/client.go\n+++ b/client.go\n@@ -45,6 +45,12 @@\n+  for attempt := 0; attempt < maxRetries; attempt++ {\n+    resp, err := c.do(req)\n+    if err == nil { return resp, nil }\n+    time.Sleep(backoff(attempt))\n+  }",
		Files:     []string{"client.go", "client_test.go"},
		Additions: 12,
		Deletions: 3,
	}, nil
}

type AddReviewCommentInput struct {
	Repo   string `json:"repo" jsonschema:"repository in owner/name format"`
	Number int    `json:"number" jsonschema:"pull request number"`
	Path   string `json:"path" jsonschema:"file path to comment on"`
	Line   int    `json:"line" jsonschema:"line number to comment on"`
	Body   string `json:"body" jsonschema:"review comment body (markdown)"`
}

type AddReviewCommentOutput struct {
	CommentID int    `json:"commentId"`
	URL       string `json:"url"`
}

func AddReviewComment(_ context.Context, _ *mcp.CallToolRequest, in AddReviewCommentInput) (*mcp.CallToolResult, AddReviewCommentOutput, error) {
	return nil, AddReviewCommentOutput{
		CommentID: 1001,
		URL:       fmt.Sprintf("https://github.com/%s/pull/%d#discussion_r1001", in.Repo, in.Number),
	}, nil
}

// -- Project Management variant tools -----------------------------------------

type ListIssuesInput struct {
	Repo   string   `json:"repo" jsonschema:"repository in owner/name format"`
	State  string   `json:"state,omitempty" jsonschema:"filter by state: open, closed, all (default: open)"`
	Labels []string `json:"labels,omitempty" jsonschema:"filter by label names"`
}

type Issue struct {
	Number   int      `json:"number"`
	Title    string   `json:"title"`
	State    string   `json:"state"`
	Labels   []string `json:"labels"`
	Assignee string   `json:"assignee,omitempty"`
}

type ListIssuesOutput struct {
	Issues []Issue `json:"issues"`
}

func ListIssues(_ context.Context, _ *mcp.CallToolRequest, in ListIssuesInput) (*mcp.CallToolResult, ListIssuesOutput, error) {
	return nil, ListIssuesOutput{
		Issues: []Issue{
			{Number: 101, Title: "API rate limiting returns wrong status code", State: "open", Labels: []string{"bug", "api"}, Assignee: "alice"},
			{Number: 98, Title: "Add support for webhook retries", State: "open", Labels: []string{"enhancement"}, Assignee: ""},
			{Number: 95, Title: "Document authentication flow", State: "open", Labels: []string{"documentation"}, Assignee: "carol"},
		},
	}, nil
}

type CreateIssueInput struct {
	Repo   string   `json:"repo" jsonschema:"repository in owner/name format"`
	Title  string   `json:"title" jsonschema:"issue title"`
	Body   string   `json:"body" jsonschema:"issue body (markdown)"`
	Labels []string `json:"labels,omitempty" jsonschema:"labels to apply"`
}

type CreateIssueOutput struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

func CreateIssue(_ context.Context, _ *mcp.CallToolRequest, in CreateIssueInput) (*mcp.CallToolResult, CreateIssueOutput, error) {
	return nil, CreateIssueOutput{
		Number: 102,
		URL:    fmt.Sprintf("https://github.com/%s/issues/102", in.Repo),
	}, nil
}

type AddLabelInput struct {
	Repo   string   `json:"repo" jsonschema:"repository in owner/name format"`
	Number int      `json:"number" jsonschema:"issue number"`
	Labels []string `json:"labels" jsonschema:"labels to add"`
}

type AddLabelOutput struct {
	Labels []string `json:"currentLabels"`
}

func AddLabel(_ context.Context, _ *mcp.CallToolRequest, in AddLabelInput) (*mcp.CallToolResult, AddLabelOutput, error) {
	return nil, AddLabelOutput{
		Labels: append([]string{"bug", "api"}, in.Labels...),
	}, nil
}

// -- Security variant tools (read-only) ---------------------------------------

type ListSecurityAlertsInput struct {
	Repo     string `json:"repo" jsonschema:"repository in owner/name format"`
	Severity string `json:"severity,omitempty" jsonschema:"filter by severity: critical, high, medium, low (default: all)"`
	State    string `json:"state,omitempty" jsonschema:"filter by state: open, dismissed, fixed (default: open)"`
}

type SecurityAlert struct {
	Number   int    `json:"number"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	State    string `json:"state"`
	Path     string `json:"path"`
}

type ListSecurityAlertsOutput struct {
	Alerts []SecurityAlert `json:"alerts"`
}

func ListSecurityAlerts(_ context.Context, _ *mcp.CallToolRequest, in ListSecurityAlertsInput) (*mcp.CallToolResult, ListSecurityAlertsOutput, error) {
	return nil, ListSecurityAlertsOutput{
		Alerts: []SecurityAlert{
			{Number: 1, Rule: "sql-injection", Severity: "critical", State: "open", Path: "src/db/query.go"},
			{Number: 2, Rule: "hardcoded-secret", Severity: "high", State: "open", Path: "config/settings.go"},
			{Number: 3, Rule: "insecure-hash", Severity: "medium", State: "open", Path: "pkg/auth/hash.go"},
		},
	}, nil
}

type GetAdvisoryInput struct {
	AdvisoryID string `json:"advisoryId" jsonschema:"security advisory identifier (e.g. GHSA-xxxx-xxxx-xxxx)"`
}

type GetAdvisoryOutput struct {
	ID          string   `json:"id"`
	Summary     string   `json:"summary"`
	Severity    string   `json:"severity"`
	CVEs        []string `json:"cves"`
	AffectedPkg string   `json:"affectedPackage"`
	PatchedIn   string   `json:"patchedIn"`
}

func GetAdvisory(_ context.Context, _ *mcp.CallToolRequest, in GetAdvisoryInput) (*mcp.CallToolResult, GetAdvisoryOutput, error) {
	return nil, GetAdvisoryOutput{
		ID:          in.AdvisoryID,
		Summary:     "Remote code execution via crafted request payload",
		Severity:    "critical",
		CVEs:        []string{"CVE-2024-12345"},
		AffectedPkg: "example.com/server",
		PatchedIn:   "v1.2.4",
	}, nil
}

// -- CI/CD variant tools ------------------------------------------------------

type ListWorkflowRunsInput struct {
	Repo     string `json:"repo" jsonschema:"repository in owner/name format"`
	Workflow string `json:"workflow,omitempty" jsonschema:"filter by workflow name or filename"`
	Status   string `json:"status,omitempty" jsonschema:"filter by status: completed, in_progress, queued (default: all)"`
}

type WorkflowRun struct {
	ID         int    `json:"id"`
	Workflow   string `json:"workflow"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion,omitempty"`
	Branch     string `json:"branch"`
}

type ListWorkflowRunsOutput struct {
	Runs []WorkflowRun `json:"runs"`
}

func ListWorkflowRuns(_ context.Context, _ *mcp.CallToolRequest, in ListWorkflowRunsInput) (*mcp.CallToolResult, ListWorkflowRunsOutput, error) {
	return nil, ListWorkflowRunsOutput{
		Runs: []WorkflowRun{
			{ID: 5001, Workflow: "ci.yml", Status: "completed", Conclusion: "success", Branch: "main"},
			{ID: 5002, Workflow: "ci.yml", Status: "in_progress", Branch: "feat/new-api"},
			{ID: 5003, Workflow: "deploy.yml", Status: "completed", Conclusion: "failure", Branch: "main"},
		},
	}, nil
}

type TriggerWorkflowInput struct {
	Repo     string            `json:"repo" jsonschema:"repository in owner/name format"`
	Workflow string            `json:"workflow" jsonschema:"workflow filename (e.g. ci.yml)"`
	Ref      string            `json:"ref" jsonschema:"git ref to run against (branch, tag, or SHA)"`
	Inputs   map[string]string `json:"inputs,omitempty" jsonschema:"workflow input parameters"`
}

type TriggerWorkflowOutput struct {
	RunID int    `json:"runId"`
	URL   string `json:"url"`
}

func TriggerWorkflow(_ context.Context, _ *mcp.CallToolRequest, in TriggerWorkflowInput) (*mcp.CallToolResult, TriggerWorkflowOutput, error) {
	return nil, TriggerWorkflowOutput{
		RunID: 5004,
		URL:   fmt.Sprintf("https://github.com/%s/actions/runs/5004", in.Repo),
	}, nil
}
