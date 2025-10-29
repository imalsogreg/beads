package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	httpserver "github.com/imalsogreg/beads/internal/http"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP REST API server",
	Long: `Start an HTTP server that exposes all beads commands via REST API.

The server provides both JSON and human-readable text responses based on
the Accept header. All endpoints (except GET /) require Bearer token
authentication via the BEADS_API_SECRET environment variable.

Example:
  # Start server on default port 8080
  bd serve

  # Start on custom port
  bd serve --port 3000

  # Start with custom database
  bd serve --db /path/to/beads.db

  # Set API secret for authentication
  export BEADS_API_SECRET=your-secret-token
  bd serve

The server will run until interrupted (Ctrl+C).`,
	RunE: runServe,
}

var (
	servePort string
	serveHost string
)

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&servePort, "port", "8080", "Port to listen on")
	serveCmd.Flags().StringVar(&serveHost, "host", "0.0.0.0", "Host to bind to")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Use the global store that was initialized in PersistentPreRun
	if store == nil {
		return fmt.Errorf("failed to initialize database")
	}

	defer store.Close()

	log.Printf("📂 Database: %s\n", dbPath)

	// Check for API secret
	if secret := os.Getenv("BEADS_API_SECRET"); secret != "" {
		log.Printf("🔒 Authentication: enabled (BEADS_API_SECRET is set)\n")
	} else {
		log.Printf("⚠️  Authentication: disabled (BEADS_API_SECRET not set - development mode)\n")
	}

	// Create HTTP server
	addr := fmt.Sprintf("%s:%s", serveHost, servePort)
	server, err := httpserver.NewServer(store, addr)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Setup graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Printf("🚀 Server starting on http://%s\n", addr)
		log.Printf("📚 API docs available at http://%s/\n", addr)
		if err := server.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case <-stop:
		log.Println("\n🛑 Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10)
		defer cancel()
		if err := server.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
		log.Println("✅ Server stopped")
	}

	return nil
}
