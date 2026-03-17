// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// backend abstracts how a variant connects to its backing MCP server.
// The in-memory implementation lives in this file; future implementations
// (HTTP, remote) will satisfy the same interface.
type backend interface {
	// connect creates a connection to the backing server and returns an
	// innerConnection for dispatching requests. frontSession may be nil
	// in stateless mode.
	connect(ctx context.Context, variant ServerVariant, frontSession *mcp.ServerSession) (*innerConnection, error)

	// capabilities performs an ephemeral connect to discover the server's
	// advertised capabilities, then tears down the probe connection.
	capabilities(ctx context.Context) (*mcp.ServerCapabilities, error)

	// close releases any resources held by the backend.
	close() error
}

// inMemoryBackend connects to a co-located *mcp.Server via in-memory
// transports.
//
// Limitation: list-changed and resource-updated notifications from
// inner servers are silently dropped. The mcp.ServerSession API only
// exposes NotifyProgress and Log; there is no public method to send
// tool/resource/prompt list-changed or resource-updated notifications
// to the front client. This means that if an inner server dynamically
// adds or removes tools/resources/prompts at runtime, the front
// client will NOT be notified.
//
// In practice this is acceptable because inner servers are typically
// statically configured (tools registered at startup). If dynamic
// capability changes are needed, this will require the Go MCP SDK to
// expose generic notification sending on ServerSession.
type inMemoryBackend struct {
	server *mcp.Server

	// serverHandler is the inner server's handler chain, captured via
	// middleware so we can bypass the transport and preserve context values.
	captureOnce      sync.Once
	mcpMethodHandler mcp.MethodHandler
}

// captureMCPMethodHandler captures a reference to the inner server's
// handler chain. This is a workaround using AddReceivingMiddleware to
// gain reference to mcp.Server.receivingMethodHandler_ since the SDK
// does not expose a public accessor for it.
// This can be replaced once the SDK exposes a public accessor for the
// receiving handler chain.
// The middleware itself is a no-op (returns next unmodified). Called
// once; the capture runs synchronously during AddReceivingMiddleware.
func (b *inMemoryBackend) captureMCPMethodHandler() {
	b.captureOnce.Do(func() {
		b.server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			b.mcpMethodHandler = next
			return next
		})
	})
}

// connect creates an in-memory transport pair and connects the inner server.
// Requests bypass the transport via serverHandler to preserve context values.
// The transport is kept alive only for notification forwarding (progress,
// logging) from the inner server to the front client.
func (b *inMemoryBackend) connect(ctx context.Context, variant ServerVariant, frontSession *mcp.ServerSession) (*innerConnection, error) {
	b.captureMCPMethodHandler()

	serverTransport, clientSideTransport := mcp.NewInMemoryTransports()

	serverSession, err := b.server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, err
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "variant-proxy-client",
		Version: "1.0.0",
	}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(ctx context.Context, req *mcp.ProgressNotificationClientRequest) {
			if frontSession != nil {
				injectVariantMeta(req.Params, variant.ID)
				_ = frontSession.NotifyProgress(ctx, req.Params)
			}
		},
		LoggingMessageHandler: func(ctx context.Context, req *mcp.LoggingMessageRequest) {
			if frontSession != nil {
				injectVariantMeta(req.Params, variant.ID)
				_ = frontSession.Log(ctx, req.Params)
			}
		},
		ToolListChangedHandler:     func(context.Context, *mcp.ToolListChangedRequest) {},
		ResourceListChangedHandler: func(context.Context, *mcp.ResourceListChangedRequest) {},
		PromptListChangedHandler:   func(context.Context, *mcp.PromptListChangedRequest) {},
		ResourceUpdatedHandler:     func(context.Context, *mcp.ResourceUpdatedNotificationRequest) {},
	})

	clientSession, err := client.Connect(ctx, clientSideTransport, nil)
	if err != nil {
		serverSession.Close()
		return nil, err
	}

	return &innerConnection{
		backendSession: &backendSession{
			variantID:        variant.ID,
			serverSession:    serverSession,
			mcpMethodHandler: b.mcpMethodHandler,
		},
		cleanupFn: func() {
			clientSession.Close()
			serverSession.Close()
		},
	}, nil
}

// capabilities performs an ephemeral in-memory connect to discover the
// server's advertised capabilities.
func (b *inMemoryBackend) capabilities(ctx context.Context) (*mcp.ServerCapabilities, error) {
	st, ct := mcp.NewInMemoryTransports()
	ss, err := b.server.Connect(ctx, st, nil)
	if err != nil {
		return nil, err
	}

	c := mcp.NewClient(&mcp.Implementation{Name: "cap-probe", Version: "1.0.0"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		ss.Close()
		return nil, err
	}

	var caps *mcp.ServerCapabilities
	if ir := cs.InitializeResult(); ir != nil {
		caps = ir.Capabilities
	}

	cs.Close()
	ss.Close()
	return caps, nil
}

// close is a no-op for in-memory backends.
func (b *inMemoryBackend) close() error {
	return nil
}
