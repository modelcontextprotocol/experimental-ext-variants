// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"

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
// The sending redirect middleware intercepts all outgoing messages from
// the inner server and forwards them through the front session. This
// covers notifications (progress, log) and server-to-client requests
// (Elicit, CreateMessage, ListRoots) during request handling.
//
// Limitation: async notifications (e.g. tool/resource/prompt
// list-changed) triggered outside request handling use
// context.Background() and lack the front session in the context, so
// the middleware falls through and these notifications are dropped.
// In practice this is acceptable because inner servers are typically
// statically configured (tools registered at startup).
type inMemoryBackend struct {
	variantID        string
	server           *mcp.Server
	mcpMethodHandler mcp.MethodHandler
}

// sessionSwappedRequest wraps an existing mcp.Request but returns a
// different session from GetSession(). The embedded mcp.Request satisfies
// the unexported isRequest() method required by the interface.
type sessionSwappedRequest struct {
	mcp.Request
	session mcp.Session
}

func (r *sessionSwappedRequest) GetSession() mcp.Session { return r.session }

// sendingRedirectMiddleware returns a sending middleware that intercepts all
// outgoing messages from the inner server (notifications and server-to-client
// requests) and redirects them through the front server's sending handler with
// the front session swapped in. By swapping the session we route messages
// through the front connection to the real client.
//
// The front session (per-request) is read from context; the sending handler
// (constant) is read from the Server struct.
func sendingRedirectMiddleware(variantID string, vs *Server) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			frontSession, _ := ctx.Value(frontSessionKeyType{}).(*mcp.ServerSession)
			if frontSession == nil || vs.frontSendingHandler == nil {
				return next(ctx, method, req)
			}
			return vs.frontSendingHandler(ctx, method, &sessionSwappedRequest{
				Request: req,
				session: frontSession,
			})
		}
	}
}

// newInMemoryBackend creates an inMemoryBackend, registers the sending
// redirect middleware on the inner server, and captures the inner
// server's handler chain for direct dispatch.
func newInMemoryBackend(server *mcp.Server, variantID string, vs *Server) *inMemoryBackend {
	server.AddSendingMiddleware(sendingRedirectMiddleware(variantID, vs))
	return &inMemoryBackend{
		variantID:        variantID,
		server:           server,
		mcpMethodHandler: captureReceivingMethodHandler(server),
	}
}

// captureReceivingMethodHandler captures and returns a reference to the inner
// server's handler chain. This is a workaround using AddReceivingMiddleware
// to gain a reference to mcp.Server.receivingMethodHandler_, since the SDK
// does not expose a public accessor for it. This can be replaced once the
// SDK exposes a public accessor for the receiving handler chain.
func captureReceivingMethodHandler(server *mcp.Server) mcp.MethodHandler {
	var handler mcp.MethodHandler
	// The middleware is identity (returns next unmodified), so the handler
	// chain is unchanged, no extra hop introduced even if called multiple times.
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		handler = next
		return next
	})

	return handler
}

// connect creates an in-memory transport pair and connects the inner server.
// Requests bypass the transport via mcpMethodHandler to preserve context
// values. Notifications are redirected by the sending middleware registered
// in newInMemoryBackend; the proxy client is kept only for the initialize
// handshake and to set the inner session's log level.
func (b *inMemoryBackend) connect(ctx context.Context, variant ServerVariant, frontSession *mcp.ServerSession) (*innerConnection, error) {
	serverTransport, clientSideTransport := mcp.NewInMemoryTransports()

	serverSession, err := b.server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, err
	}

	// The proxy client completes the initialize handshake and sets the
	// inner session's log level. It must remain open for the lifetime of
	// the serverSession — closing it tears down the in-memory transport,
	// causing the serverSession to shut down. The nop handlers advertise
	// capabilities so the inner ServerSession doesn't short-circuit
	// methods like Elicit with "client does not support X". They are
	// never called because the sending redirect middleware intercepts
	// all outgoing messages before they reach the transport.
	nopElicit := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, nil
	}
	nopSampling := func(context.Context, *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
		return nil, nil
	}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "variant-proxy-client",
		Version: "1.0.0",
	}, &mcp.ClientOptions{
		ElicitationHandler:   nopElicit,
		CreateMessageHandler: nopSampling,
	})

	clientSession, err := client.Connect(ctx, clientSideTransport, nil)
	if err != nil {
		serverSession.Close()
		return nil, err
	}

	// Set the inner session's log level to the lowest so that
	// ServerSession.Log() does not short-circuit before reaching the
	// sending middleware. The actual log-level filtering is performed by
	// the front-facing session when the middleware redirects the
	// notification. Errors are ignored: if the inner server does not
	// advertise the Logging capability the call simply fails harmlessly.
	_ = clientSession.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "debug"})

	return &innerConnection{
		backendSession: &backendSession{
			variantID:        b.variantID,
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
