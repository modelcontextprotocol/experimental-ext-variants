// Example: A developer productivity platform (à la GitHub) that exposes
// different tool sets for different agent types using MCP server variants,
// served over HTTP for multiple concurrent clients.
//
// Variants:
//   - code-review: PR operations, diffs, and review comments
//   - project-management: issue tracking, labels, and assignments
//   - security-readonly: security scanning alerts and advisories (read-only)
//   - ci-automation: CI/CD workflow management and dispatch
//
// Run:
//
//	go run .
//
// Then connect any MCP client to http://localhost:8080.
package main

import (
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/modelcontextprotocol/experimental-ext-variants/go/sdk/examples/server/exampletools"
	"github.com/modelcontextprotocol/experimental-ext-variants/go/sdk/variants"
)

func main() {
	// Code review variant: PR-focused tools for code review agents
	codeReviewServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
	mcp.AddTool(codeReviewServer, &mcp.Tool{
		Name:        "list_pull_requests",
		Description: "List open pull requests, optionally filtered by author",
	}, exampletools.ListPullRequests)
	mcp.AddTool(codeReviewServer, &mcp.Tool{
		Name:        "get_diff",
		Description: "Get the diff for a pull request, including changed files and line counts",
	}, exampletools.GetDiff)
	mcp.AddTool(codeReviewServer, &mcp.Tool{
		Name:        "add_review_comment",
		Description: "Post a review comment on a specific line of a pull request",
	}, exampletools.AddReviewComment)

	// Project management variant: issue-focused tools for PM agents
	pmServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
	mcp.AddTool(pmServer, &mcp.Tool{
		Name:        "list_issues",
		Description: "List issues, optionally filtered by state and labels",
	}, exampletools.ListIssues)
	mcp.AddTool(pmServer, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create a new issue with title, body, and optional labels",
	}, exampletools.CreateIssue)
	mcp.AddTool(pmServer, &mcp.Tool{
		Name:        "add_label",
		Description: "Add labels to an existing issue",
	}, exampletools.AddLabel)

	// Security variant: read-only security scanning tools
	securityServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
	mcp.AddTool(securityServer, &mcp.Tool{
		Name:        "list_security_alerts",
		Description: "List code scanning alerts for a repository",
	}, exampletools.ListSecurityAlerts)
	mcp.AddTool(securityServer, &mcp.Tool{
		Name:        "get_advisory",
		Description: "Get details of a security advisory",
	}, exampletools.GetAdvisory)

	// CI/CD variant: workflow management tools for automation agents
	ciServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
	mcp.AddTool(ciServer, &mcp.Tool{
		Name:        "list_workflow_runs",
		Description: "List recent workflow runs for a repository",
	}, exampletools.ListWorkflowRuns)
	mcp.AddTool(ciServer, &mcp.Tool{
		Name:        "trigger_workflow",
		Description: "Trigger a workflow dispatch event",
	}, exampletools.TriggerWorkflow)

	vs := variants.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}).
		WithVariant(variants.ServerVariant{
			ID:          "code-review",
			Description: "Pull request and code review operations. Includes diff viewing, review comments, approval workflows, and merge controls.",
			Hints:       map[string]string{"domain": "code-review", "accessLevel": "read-write"},
			Status:      variants.Stable,
		}, codeReviewServer, 0).
		WithVariant(variants.ServerVariant{
			ID:          "project-management",
			Description: "Issue and project tracking operations. Includes issue CRUD, labels, milestones, assignments, and project boards.",
			Hints:       map[string]string{"domain": "project-management", "accessLevel": "read-write"},
			Status:      variants.Stable,
		}, pmServer, 1).
		WithVariant(variants.ServerVariant{
			ID:          "security-readonly",
			Description: "Security scanning and vulnerability management. Read-only access to code scanning alerts, secret detection, and security advisories.",
			Hints:       map[string]string{"domain": "security", "accessLevel": "readonly"},
			Status:      variants.Stable,
		}, securityServer, 2).
		WithVariant(variants.ServerVariant{
			ID:          "ci-automation",
			Description: "CI/CD workflow management. Trigger runs, monitor jobs, manage deployments designed for automation agents.",
			Hints:       map[string]string{"domain": "ci-cd", "accessLevel": "automation"},
			Status:      variants.Stable,
		}, ciServer, 3)

	handler := variants.NewStreamableHTTPHandler(vs, nil)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
