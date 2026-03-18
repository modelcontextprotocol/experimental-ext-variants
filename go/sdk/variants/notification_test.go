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

// frontSessionKey is a test-local context key for the front-facing session.
type frontSessionKey struct{}

// frontSessionFromCtx extracts the front-facing ServerSession that was
// injected into the context by middleware on the outer server.
func frontSessionFromCtx(ctx context.Context) *mcp.ServerSession {
	s, _ := ctx.Value(frontSessionKey{}).(*mcp.ServerSession)
	return s
}

const notifyCount = 3

type notifyInput struct {
	Count int `json:"count"`
}

// notifyFixedHandler sends notifications via the front session from context.
func notifyFixedHandler(ctx context.Context, req *mcp.CallToolRequest, input notifyInput) (*mcp.CallToolResult, any, error) {
	count := input.Count
	if count <= 0 {
		count = notifyCount
	}

	session := frontSessionFromCtx(ctx)
	if session == nil {
		session = req.Session
	}

	progressToken := req.Params.GetProgressToken()

	for i := 1; i <= count; i++ {
		session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: progressToken,
			Progress:      float64(i),
			Total:         float64(count),
			Message:       fmt.Sprintf("fixed progress %d/%d", i, count),
		})

		session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  "info",
			Logger: "notify-fixed",
			Data:   fmt.Sprintf("fixed log %d/%d", i, count),
		})

		time.Sleep(10 * time.Millisecond)
	}

	return nil, map[string]string{"status": "done"}, nil
}

// notifyBrokenHandler sends notifications via req.Session (inner session).
// This demonstrates the bug: log messages are dropped because the inner
// session has no log level set.
func notifyBrokenHandler(ctx context.Context, req *mcp.CallToolRequest, input notifyInput) (*mcp.CallToolResult, any, error) {
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
			Message:       fmt.Sprintf("broken progress %d/%d", i, count),
		})

		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  "info",
			Logger: "notify-broken",
			Data:   fmt.Sprintf("broken log %d/%d", i, count),
		})

		time.Sleep(10 * time.Millisecond)
	}

	return nil, map[string]string{"status": "done"}, nil
}

// newNotifyInnerServer creates an inner mcp.Server with both tools.
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
		Name:        "notify_fixed",
		Description: "Sends notifications via front session from context",
	}, notifyFixedHandler)

	mcp.AddTool(inner, &mcp.Tool{
		Name:        "notify_broken",
		Description: "Sends notifications via req.Session (inner session)",
	}, notifyBrokenHandler)

	return inner
}

// notificationCollector collects progress and log notifications from the client.
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

// connectNotifyTestClient builds the variant server, registers middleware on
// the outer mcp.Server to inject the front session into context, then
// connects a test client. No SDK code is modified.
func connectNotifyTestClient(t *testing.T, vs *Server, clientOpts *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	// Get the outer mcp.Server from the variant server.
	frontServer, err := vs.mcpServer(false)
	require.NoError(t, err)

	// Register middleware that injects the front-facing session into context.
	// This runs outermost (before sessionMiddleware), so the enriched ctx
	// flows through dispatch → backendSession → inner server handler chain.
	frontServer.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			ctx = context.WithValue(ctx, frontSessionKey{}, req.GetSession())
			return next(ctx, method, req)
		}
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	errCh := make(chan error, 1)
	go func() {
		errCh <- frontServer.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "notify-test-client", Version: "v0.0.1"},
		clientOpts,
	)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("client.Connect: %v", err)
	}

	t.Cleanup(func() {
		session.Close()
		cancel()
		<-errCh
	})

	return session
}

func TestNotification_FixedToolDeliversAll(t *testing.T) {
	inner := newNotifyInnerServer()
	vs := NewServer(&mcp.Implementation{Name: "notify-test", Version: "v1.0.0"}).
		WithVariant(ServerVariant{
			ID:          "default",
			Description: "Notification test variant",
			Status:      Stable,
		}, inner, 0)

	collector := &notificationCollector{}
	session := connectNotifyTestClient(t, vs, collector.clientOptions())
	ctx := context.Background()

	// Set log level so the front session will forward log messages.
	err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "debug"})
	require.NoError(t, err)

	// Call the fixed tool.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "notify_fixed",
		Arguments: map[string]json.RawMessage{"count": json.RawMessage(`3`)},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Give forwarded notifications a moment to arrive.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, notifyCount, collector.progressCount(),
		"all progress notifications should be delivered via front session")
	assert.Equal(t, notifyCount, collector.logCount(),
		"all log messages should be delivered via front session")
}

func TestNotification_BrokenToolDropsLogs(t *testing.T) {
	inner := newNotifyInnerServer()
	vs := NewServer(&mcp.Implementation{Name: "notify-test", Version: "v1.0.0"}).
		WithVariant(ServerVariant{
			ID:          "default",
			Description: "Notification test variant",
			Status:      Stable,
		}, inner, 0)

	collector := &notificationCollector{}
	session := connectNotifyTestClient(t, vs, collector.clientOptions())
	ctx := context.Background()

	// Set log level on the front session.
	err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "debug"})
	require.NoError(t, err)

	// Call the broken tool (uses req.Session = inner session).
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "notify_broken",
		Arguments: map[string]json.RawMessage{"count": json.RawMessage(`3`)},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Give forwarded notifications a moment to arrive.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 0, collector.logCount(),
		"log messages should be dropped because inner session has no log level set")

	// Progress notifications may or may not arrive due to race condition.
	t.Logf("progress notifications received (non-deterministic): %d/%d",
		collector.progressCount(), notifyCount)
}
