// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const notifyCount = 3

type notifyInput struct {
	ClientID string `json:"client_id"`
	Count    int    `json:"count"`
}

func notifyHandler(ctx context.Context, req *mcp.CallToolRequest, input notifyInput) (*mcp.CallToolResult, any, error) {
	count := input.Count
	if count <= 0 {
		count = notifyCount
	}

	progressToken := req.Params.GetProgressToken()

	for i := 1; i <= count; i++ {
		req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: progressToken,
			Progress:      float64(i),
			Total:         float64(count),
			Message:       fmt.Sprintf("[%s] progress %d/%d", input.ClientID, i, count),
		})
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  "info",
			Logger: "notify-test",
			Data:   fmt.Sprintf("[%s] log %d/%d", input.ClientID, i, count),
		})
	}

	return nil, map[string]string{"status": "done", "client_id": input.ClientID}, nil
}

func newNotifyVariantServer() *Server {
	inner := mcp.NewServer(
		&mcp.Implementation{Name: "notify-test", Version: "v1.0.0"},
		&mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{
				Logging: &mcp.LoggingCapabilities{},
			},
		},
	)
	mcp.AddTool(inner, &mcp.Tool{
		Name:        "notify",
		Description: "Sends progress and log notifications via req.Session",
	}, notifyHandler)

	return NewServer(&mcp.Implementation{Name: "notify-test", Version: "v1.0.0"}).
		WithVariant(ServerVariant{
			ID:          "default",
			Description: "Notification test variant",
			Status:      Stable,
		}, inner, 0)
}

type notificationCollector struct {
	mu       sync.Mutex
	progress []*mcp.ProgressNotificationParams
	logs     []*mcp.LoggingMessageParams
}

func (c *notificationCollector) clientOptions() *mcp.ClientOptions {
	return &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.progress = append(c.progress, req.Params)
		},
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.logs = append(c.logs, req.Params)
		},
	}
}

func (c *notificationCollector) progressCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.progress)
}

func (c *notificationCollector) logCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.logs)
}

const numClients = 3

func TestNotificationDelivery(t *testing.T) {
	tests := []struct {
		name      string
		stateless bool
	}{
		{name: "Stateful", stateless: false},
		{name: "Stateless", stateless: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := newNotifyVariantServer()
			ctx := context.Background()

			handler := NewStreamableHTTPHandler(vs, &mcp.StreamableHTTPOptions{Stateless: tt.stateless})
			httpSrv := httptest.NewServer(handler)
			t.Cleanup(httpSrv.Close)

			// Set up N clients, each with its own collector.
			type clientState struct {
				id        string
				session   *mcp.ClientSession
				collector *notificationCollector
			}
			clients := make([]clientState, numClients)
			for i := range clients {
				cs := &clients[i]
				cs.id = fmt.Sprintf("client-%d", i)
				cs.collector = &notificationCollector{}

				client := mcp.NewClient(
					&mcp.Implementation{Name: cs.id, Version: "v0.0.1"},
					cs.collector.clientOptions(),
				)
				var err error
				cs.session, err = client.Connect(ctx, &mcp.StreamableClientTransport{
					Endpoint: httpSrv.URL,
				}, nil)
				require.NoError(t, err)
				t.Cleanup(func() { cs.session.Close() })

				err = cs.session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "debug"})
				require.NoError(t, err)
			}

			// All clients call the tool concurrently.
			var wg sync.WaitGroup
			errs := make([]error, numClients)
			for i := range clients {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					cs := &clients[idx]
					args := map[string]json.RawMessage{
						"client_id": json.RawMessage(fmt.Sprintf("%q", cs.id)),
						"count":     json.RawMessage(fmt.Sprintf("%d", notifyCount)),
					}
					result, err := cs.session.CallTool(ctx, &mcp.CallToolParams{
						Name:      "notify",
						Arguments: args,
					})
					if err == nil && result == nil {
						err = fmt.Errorf("nil result for %s", cs.id)
					}
					errs[idx] = err
				}(i)
			}
			wg.Wait()

			for i, err := range errs {
				require.NoError(t, err, "client %d tool call failed", i)
			}

			// Wait for all notifications to arrive at each collector.
			for i := range clients {
				cs := &clients[i]
				require.Eventually(t, func() bool {
					return cs.collector.progressCount() == notifyCount && cs.collector.logCount() == notifyCount
				}, 2*time.Second, 10*time.Millisecond,
					"client %s: expected %d progress and %d log notifications", cs.id, notifyCount, notifyCount)
			}

			// Verify each client received only its own notifications.
			for i := range clients {
				cs := &clients[i]
				cs.collector.mu.Lock()

				for j := 0; j < notifyCount; j++ {
					p := cs.collector.progress[j]
					assert.Equal(t, float64(j+1), p.Progress)
					assert.Equal(t, float64(notifyCount), p.Total)
					assert.Equal(t, fmt.Sprintf("[%s] progress %d/%d", cs.id, j+1, notifyCount), p.Message,
						"client %s got a progress notification belonging to another client", cs.id)

					l := cs.collector.logs[j]
					assert.Equal(t, "info", string(l.Level))
					assert.Equal(t, "notify-test", l.Logger)
					assert.Equal(t, fmt.Sprintf("[%s] log %d/%d", cs.id, j+1, notifyCount), l.Data,
						"client %s got a log notification belonging to another client", cs.id)
				}

				cs.collector.mu.Unlock()
			}
		})
	}
}
