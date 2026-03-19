package variants

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const notifyCount = 3

type notifyInput struct {
	Count int `json:"count"`
}

// notifyHandler sends progress notifications and log messages via
// req.Session. The sending redirect middleware on the inner server
// intercepts these and routes them through the front session.
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
			Message:       fmt.Sprintf("progress %d/%d", i, count),
		})
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  "info",
			Logger: "notify-test",
			Data:   fmt.Sprintf("log %d/%d", i, count),
		})
		time.Sleep(10 * time.Millisecond)
	}

	return nil, map[string]string{"status": "done"}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newNotifyInnerServer() *mcp.Server {
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

	return inner
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

func newNotifyVariantServer() *Server {
	return NewServer(&mcp.Implementation{Name: "notify-test", Version: "v1.0.0"}).
		WithVariant(ServerVariant{
			ID:          "default",
			Description: "Notification test variant",
			Status:      Stable,
		}, newNotifyInnerServer(), 0)
}

// ---------------------------------------------------------------------------
// Test
// ---------------------------------------------------------------------------

func TestNotificationDelivery(t *testing.T) {
	vs := newNotifyVariantServer()
	collector := &notificationCollector{}
	session := connectTestClient(t, vs, collector.clientOptions())
	ctx := context.Background()

	err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "debug"})
	require.NoError(t, err)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "notify",
		Arguments: map[string]json.RawMessage{"count": json.RawMessage(`3`)},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, notifyCount, collector.progressCount(),
		"all progress notifications should arrive via sending redirect middleware")
	assert.Equal(t, notifyCount, collector.logCount(),
		"all log messages should arrive via sending redirect middleware")
}
