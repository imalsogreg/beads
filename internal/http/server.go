package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/imalsogreg/beads/internal/rpc"
	"github.com/imalsogreg/beads/internal/storage"
	"github.com/imalsogreg/beads/internal/types"
)

// Server wraps storage with HTTP endpoints
type Server struct {
	storage    storage.Storage
	httpServer *http.Server
	router     *mux.Router
}

// NewServer creates a new HTTP server
func NewServer(store storage.Storage, addr string) (*Server, error) {
	s := &Server{
		storage: store,
		router:  mux.NewRouter(),
	}

	s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// setupRoutes configures all HTTP endpoints
func (s *Server) setupRoutes() {
	// Apply auth middleware to all routes
	s.router.Use(s.authMiddleware)

	// API documentation
	s.router.HandleFunc("/", s.handleDocs).Methods("GET")

	// Diagnostics
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/ping", s.handlePing).Methods("GET")
	s.router.HandleFunc("/status", s.handleStatus).Methods("GET")
	s.router.HandleFunc("/metrics", s.handleMetrics).Methods("GET")

	// Issues
	s.router.HandleFunc("/issues", s.handleCreateIssue).Methods("POST")
	s.router.HandleFunc("/issues", s.handleListIssues).Methods("GET")
	s.router.HandleFunc("/issues/{id}", s.handleShowIssue).Methods("GET")
	s.router.HandleFunc("/issues/{id}", s.handleUpdateIssue).Methods("PATCH")
	s.router.HandleFunc("/issues/{id}/close", s.handleCloseIssue).Methods("POST")
	s.router.HandleFunc("/issues/ready", s.handleReadyWork).Methods("GET")
	s.router.HandleFunc("/issues/stats", s.handleStats).Methods("GET")

	// Comments
	s.router.HandleFunc("/issues/{id}/comments", s.handleAddComment).Methods("POST")
	s.router.HandleFunc("/issues/{id}/comments", s.handleListComments).Methods("GET")

	// Labels
	s.router.HandleFunc("/issues/{id}/labels", s.handleAddLabel).Methods("POST")
	s.router.HandleFunc("/issues/{id}/labels/{label}", s.handleRemoveLabel).Methods("DELETE")

	// Dependencies
	s.router.HandleFunc("/issues/{id}/dependencies", s.handleAddDependency).Methods("POST")
	s.router.HandleFunc("/issues/{id}/dependencies/{depId}", s.handleRemoveDependency).Methods("DELETE")
	s.router.HandleFunc("/issues/{id}/tree", s.handleDependencyTree).Methods("GET")

	// Epics
	s.router.HandleFunc("/epics/{id}/status", s.handleEpicStatus).Methods("GET")

	// Compaction
	s.router.HandleFunc("/compact", s.handleCompact).Methods("POST")
	s.router.HandleFunc("/compact/stats", s.handleCompactStats).Methods("GET")

	// Import/Export
	s.router.HandleFunc("/export", s.handleExport).Methods("POST")
	s.router.HandleFunc("/import", s.handleImport).Methods("POST")

	// Batch operations
	s.router.HandleFunc("/batch", s.handleBatch).Methods("POST")

	// Config endpoints
	s.router.HandleFunc("/config/{key}", s.handleGetConfig).Methods("GET")
	s.router.HandleFunc("/config/{key}", s.handleSetConfig).Methods("PUT")
}

// writeSuccess writes a successful response with content negotiation
func (s *Server) writeSuccess(w http.ResponseWriter, r *http.Request, data interface{}, operation string) {
	wantsJSON := s.wantsJSON(r)

	if wantsJSON {
		// Return raw JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(data)
	} else {
		// Marshal to JSON first, then format
		dataJSON, _ := json.Marshal(data)
		formatted := s.formatResponse(operation, dataJSON)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, formatted)
	}
}

// wantsJSON determines if the client wants JSON response
func (s *Server) wantsJSON(r *http.Request) bool {
	// Check Accept header
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return true
	}
	// Default to text unless explicitly requesting JSON
	return false
}

// getActor extracts the actor from request (header, query param, or default)
func (s *Server) getActor(r *http.Request) string {
	// Check X-Actor header
	if actor := r.Header.Get("X-Actor"); actor != "" {
		return actor
	}
	// Check query param
	if actor := r.URL.Query().Get("actor"); actor != "" {
		return actor
	}
	// Default to "http-user"
	return "http-user"
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, statusCode int, err error) {
	if s.wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   err.Error(),
			"success": false,
		})
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, "Error: %s\n", err.Error())
	}
}

// formatResponse formats RPC response data as human-readable text
func (s *Server) formatResponse(operation string, data json.RawMessage) string {
	switch operation {
	case rpc.OpCreate:
		var issue types.Issue
		if err := json.Unmarshal(data, &issue); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatIssue(&issue)

	case rpc.OpList:
		var issues []*types.Issue
		if err := json.Unmarshal(data, &issues); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatIssueList(issues)

	case rpc.OpShow:
		var issue types.Issue
		if err := json.Unmarshal(data, &issue); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatIssueDetail(&issue)

	case rpc.OpReady:
		var issues []*types.Issue
		if err := json.Unmarshal(data, &issues); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatReadyWork(issues)

	case rpc.OpStats:
		var stats types.Statistics
		if err := json.Unmarshal(data, &stats); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatStats(&stats)

	case rpc.OpEpicStatus:
		var statuses []*types.EpicStatus
		if err := json.Unmarshal(data, &statuses); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatEpicStatus(statuses)

	case rpc.OpDepTree:
		var tree []*types.TreeNode
		if err := json.Unmarshal(data, &tree); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatDependencyTree(tree)

	case rpc.OpCommentList:
		var comments []*types.Comment
		if err := json.Unmarshal(data, &comments); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatComments(comments)

	case rpc.OpHealth:
		var health rpc.HealthResponse
		if err := json.Unmarshal(data, &health); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatHealth(&health)

	case rpc.OpStatus:
		var status rpc.StatusResponse
		if err := json.Unmarshal(data, &status); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatStatus(&status)

	case rpc.OpMetrics:
		var metrics rpc.MetricsSnapshot
		if err := json.Unmarshal(data, &metrics); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatMetrics(&metrics)

	case rpc.OpCompactStats:
		var stats rpc.CompactStatsData
		if err := json.Unmarshal(data, &stats); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		return s.formatCompactStats(&stats)

	default:
		// For operations that just return success (update, close, label ops, etc.)
		return "Success\n"
	}
}

// parseBody reads and parses JSON body into the target struct
func (s *Server) parseBody(r *http.Request, target interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}
	if len(body) == 0 {
		// Empty body is OK for some endpoints
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	return nil
}
