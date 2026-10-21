package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cnosuke/mcp-gemini-grounded-search/config"
	"github.com/cnosuke/mcp-gemini-grounded-search/lifecycle"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// RunHTTP starts the MCP server with Streamable HTTP transport.
func RunHTTP(cfg *config.Config, name string, version string, revision string) error {
	if cfg.HTTP.AuthToken == "" {
		return fmt.Errorf("HTTP_AUTH_TOKEN is required; unauthenticated HTTP serving is disabled")
	}
	zap.S().Infow("starting MCP Gemini Grounded Search Server (HTTP)")

	s, _, err := createMCPServer(cfg, name, version, revision)
	if err != nil {
		return err
	}

	opts := []mcpserver.StreamableHTTPOption{}
	if cfg.HTTP.HeartbeatSeconds > 0 {
		opts = append(opts, mcpserver.WithHeartbeatInterval(time.Duration(cfg.HTTP.HeartbeatSeconds)*time.Second))
	}

	httpServer := mcpserver.NewStreamableHTTPServer(s, opts...)

	// Apply middleware only to MCP handler so /health bypasses auth/CORS checks.
	// Order: withOriginValidation (outer) → withAuthMiddleware (inner) → httpServer
	// This allows CORS preflight (OPTIONS without Authorization) to be handled
	// by withOriginValidation before reaching the auth check.
	var mcpHandler http.Handler = httpServer
	mcpHandler = withAuthMiddleware(mcpHandler, cfg.HTTP.AuthToken)
	if path := os.Getenv("GEMINI_AUTH_STATE_DPAPI"); path != "" {
		// Replacing the encrypted snapshot changes only future admissions.
		mcpHandler = lifecycle.AuthGuard(httpServer, func() (lifecycle.AuthSnapshot, error) {
			return lifecycle.ReadAuth(path)
		}, os.Getenv("GEMINI_AUTH_OWNER_SID"), time.Now)
	}
	mcpHandler = lifecycle.TrackRequests(mcpHandler, os.Getenv("GEMINI_INFLIGHT_DIR"))
	mcpHandler = withOriginValidation(mcpHandler, cfg.HTTP.AllowedOrigins)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.Handle(cfg.HTTP.EndpointPath, mcpHandler)

	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", cfg.HTTP.Port),
		Handler:           withHostValidation(mux, fmt.Sprintf("127.0.0.1:%d", cfg.HTTP.Port)),
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		zap.S().Infow("HTTP server listening", "addr", srv.Addr, "endpoint", cfg.HTTP.EndpointPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case sig := <-quit:
		zap.S().Infow("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		return err
	}

	// Use separate contexts to avoid shared timeout depletion between the two shutdowns.
	mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer mcpCancel()
	if err := httpServer.Shutdown(mcpCtx); err != nil {
		zap.S().Errorw("MCP server shutdown error", "error", err)
	}

	srvCtx, srvCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer srvCancel()
	if err := srv.Shutdown(srvCtx); err != nil {
		zap.S().Errorw("HTTP server shutdown error", "error", err)
		return err
	}

	zap.S().Infow("server shut down gracefully")
	return nil
}
