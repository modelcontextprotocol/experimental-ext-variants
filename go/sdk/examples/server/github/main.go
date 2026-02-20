// Example: GitHub MCP Server — a developer productivity platform that exposes
// different tool sets for different agent types using MCP server variants.
// This mirrors the GitHub MCP Server pattern described in SEP-2053's Prior Art
// section, where a single server exposes code review, project management,
// security, and CI/CD tools as separate variants with custom ranking.
//
// Capability demonstrated: Different tool sets per variant, custom ranking.
//
// Variants:
//   - code-review: PR operations, diffs, and review comments
//   - project-management: issue tracking, labels, and assignments
//   - security-readonly: security scanning alerts and advisories (read-only)
//   - ci-automation: CI/CD workflow management and dispatch
//
// Run:
//
//	go run ./examples/server/github
//
// Then connect any MCP client to http://localhost:8080.
package main

import (
	"context"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/modelcontextprotocol/experimental-ext-variants/go/sdk/variants"
)

func main() {
	// Code review variant: PR-focused tools for code review agents
	codeReviewServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
	mcp.AddTool(codeReviewServer, &mcp.Tool{
		Name:        "list_pull_requests",
		Description: "List open pull requests, optionally filtered by author",
	}, listPullRequests)
	mcp.AddTool(codeReviewServer, &mcp.Tool{
		Name:        "get_diff",
		Description: "Get the diff for a pull request, including changed files and line counts",
	}, getDiff)
	mcp.AddTool(codeReviewServer, &mcp.Tool{
		Name:        "add_review_comment",
		Description: "Post a review comment on a specific line of a pull request",
	}, addReviewComment)

	// Project management variant: issue-focused tools for PM agents
	pmServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
	mcp.AddTool(pmServer, &mcp.Tool{
		Name:        "list_issues",
		Description: "List issues, optionally filtered by state and labels",
	}, listIssues)
	mcp.AddTool(pmServer, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create a new issue with title, body, and optional labels",
	}, createIssue)
	mcp.AddTool(pmServer, &mcp.Tool{
		Name:        "add_label",
		Description: "Add labels to an existing issue",
	}, addLabel)

	// Security variant: read-only security scanning tools
	securityServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
	mcp.AddTool(securityServer, &mcp.Tool{
		Name:        "list_security_alerts",
		Description: "List code scanning alerts for a repository",
	}, listSecurityAlerts)
	mcp.AddTool(securityServer, &mcp.Tool{
		Name:        "get_advisory",
		Description: "Get details of a security advisory",
	}, getAdvisory)

	// CI/CD variant: workflow management tools for automation agents
	ciServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
	mcp.AddTool(ciServer, &mcp.Tool{
		Name:        "list_workflow_runs",
		Description: "List recent workflow runs for a repository",
	}, listWorkflowRuns)
	mcp.AddTool(ciServer, &mcp.Tool{
		Name:        "trigger_workflow",
		Description: "Trigger a workflow dispatch event",
	}, triggerWorkflow)

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
		}, ciServer, 3).
		// Custom ranking: boost variants whose "domain" hint matches the client's.
		WithRanking(func(_ context.Context, hints variants.VariantHints, vs []variants.ServerVariant) []variants.ServerVariant {
			requested, _ := variants.HintValue[string](hints, "domain")
			slices.SortStableFunc(vs, func(a, b variants.ServerVariant) int {
				aMatch := strings.Contains(strings.ToLower(a.Hints["domain"]), strings.ToLower(requested))
				bMatch := strings.Contains(strings.ToLower(b.Hints["domain"]), strings.ToLower(requested))
				if aMatch != bMatch {
					if aMatch {
						return -1
					}
					return 1
				}
				return a.Priority() - b.Priority()
			})
			return vs
		})

	handler := variants.NewStreamableHTTPHandler(vs, nil)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
