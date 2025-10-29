package http

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/imalsogreg/beads/internal/rpc"
	"github.com/imalsogreg/beads/internal/types"
)

// handleDocs serves API documentation at /
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	docs := `BEADS REST API

Base URL: /

AUTHENTICATION
  All requests (except GET /) require Bearer token authentication.

  To authenticate:
    1. Get the API secret from your team
    2. Set environment variable: export BEADS_API_SECRET=<secret>
    3. Include header in all requests:
       Authorization: Bearer $BEADS_API_SECRET

  Example:
    curl -H "Authorization: Bearer your-secret-token" \
         http://api.example.com/issues

  For agents/scripts:
    - Read token from environment variable: BEADS_API_SECRET
    - Send it in Authorization header: Authorization: Bearer <token>

  Actor tracking (optional):
    Include actor name for audit trail via:
    - Header: X-Actor: username
    - Query param: ?actor=username
    - Default: "http-user"

CONTENT NEGOTIATION
  - Accept: application/json → JSON response
  - Accept: text/plain → Human-readable text (default)

CORE ENDPOINTS

  GET  /health                        Health check
  GET  /ping                          Ping server

  POST /issues                        Create issue
       Body: {"title": "...", "description": "...", "issue_type": "task",
              "priority": 0, "assignee": "..."}

  GET  /issues                        List issues
       Query params: status, priority, assignee, type, label, limit

  GET  /issues/{id}                   Show issue details

  PATCH /issues/{id}                  Update issue
        Body: {"title": "...", "status": "...", "priority": 0, ...}

  GET  /issues/stats                  Database statistics

CONFIGURATION
  GET  /config/{key}                  Get config value (e.g., issue_prefix)
  PUT  /config/{key}                  Set config value
       Body: {"value": "..."}

EXAMPLES

  Get current prefix:
    curl -H "Authorization: Bearer $BEADS_API_SECRET" \
      http://localhost:8080/config/issue_prefix

  Create an issue:
    curl -X POST http://localhost:8080/issues \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $BEADS_API_SECRET" \
      -H "X-Actor: alice" \
      -d '{"title":"Fix login bug","issue_type":"bug","priority":0}'

  List open issues (JSON):
    curl -H "Accept: application/json" \
      -H "Authorization: Bearer $BEADS_API_SECRET" \
      "http://localhost:8080/issues?status=open"

  Show issue details (text):
    curl -H "Authorization: Bearer $BEADS_API_SECRET" \
      http://localhost:8080/issues/bd-1
`
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(docs))
}

// handlePing handles ping requests
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	result := map[string]string{
		"message": "pong",
		"status":  "ok",
	}
	s.writeSuccess(w, r, result, "ping")
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Simple health check - try to get stats from DB
	_, err := s.storage.GetStatistics(ctx)

	health := map[string]interface{}{
		"status":  "healthy",
		"message": "OK",
	}

	if err != nil {
		health["status"] = "unhealthy"
		health["message"] = err.Error()
		s.writeError(w, r, http.StatusServiceUnavailable, err)
		return
	}

	s.writeSuccess(w, r, health, "health")
}

// handleStats handles GET /issues/stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := s.storage.GetStatistics(ctx)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, stats, rpc.OpStats)
}

// handleCreateIssue handles POST /issues
func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := s.getActor(r)

	var args rpc.CreateArgs
	if err := s.parseBody(r, &args); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Convert args to Issue
	issue := &types.Issue{
		ID:                 args.ID,
		Title:              args.Title,
		Description:        args.Description,
		Design:             args.Design,
		AcceptanceCriteria: args.AcceptanceCriteria,
		IssueType:          types.IssueType(args.IssueType),
		Priority:           args.Priority,
		Status:             types.StatusOpen, // Default to "open"
	}

	if args.Assignee != "" {
		issue.Assignee = args.Assignee
	}

	// Create the issue
	if err := s.storage.CreateIssue(ctx, issue, actor); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, issue, rpc.OpCreate)
}

// handleListIssues handles GET /issues
func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	// Build filter from query params
	filter := types.IssueFilter{}

	if status := query.Get("status"); status != "" {
		s := types.Status(status)
		filter.Status = &s
	}
	if priority := query.Get("priority"); priority != "" {
		p, _ := strconv.Atoi(priority)
		filter.Priority = &p
	}
	if assignee := query.Get("assignee"); assignee != "" {
		filter.Assignee = &assignee
	}
	if issueType := query.Get("type"); issueType != "" {
		t := types.IssueType(issueType)
		filter.IssueType = &t
	}
	if label := query.Get("label"); label != "" {
		filter.Labels = []string{label}
	}
	if limit := query.Get("limit"); limit != "" {
		l, _ := strconv.Atoi(limit)
		filter.Limit = l
	}
	if q := query.Get("q"); q != "" {
		filter.TitleSearch = q
	}

	issues, err := s.storage.SearchIssues(ctx, "", filter)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, issues, rpc.OpList)
}

// handleShowIssue handles GET /issues/{id}
func (s *Server) handleShowIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	issue, err := s.storage.GetIssue(ctx, vars["id"])
	if err != nil {
		s.writeError(w, r, http.StatusNotFound, err)
		return
	}

	s.writeSuccess(w, r, issue, rpc.OpShow)
}

// handleUpdateIssue handles PATCH /issues/{id}
func (s *Server) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := s.getActor(r)
	vars := mux.Vars(r)

	// Parse the update args
	var updates map[string]interface{}
	if err := s.parseBody(r, &updates); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Update the issue
	if err := s.storage.UpdateIssue(ctx, vars["id"], updates, actor); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	// Get the updated issue
	issue, err := s.storage.GetIssue(ctx, vars["id"])
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, issue, rpc.OpUpdate)
}

// Placeholder handlers for other endpoints
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.writeSuccess(w, r, map[string]string{"status": "ok"}, "status")
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.writeSuccess(w, r, map[string]string{"message": "not implemented"}, "metrics")
}

func (s *Server) handleCloseIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := s.getActor(r)
	vars := mux.Vars(r)

	var body struct {
		Reason string `json:"reason"`
	}
	s.parseBody(r, &body)

	if err := s.storage.CloseIssue(ctx, vars["id"], body.Reason, actor); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, map[string]string{"message": "closed"}, "close")
}

func (s *Server) handleReadyWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	filter := types.WorkFilter{Status: types.StatusOpen}

	issues, err := s.storage.GetReadyWork(ctx, filter)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, issues, rpc.OpReady)
}

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := s.getActor(r)
	vars := mux.Vars(r)

	var body struct {
		Text string `json:"text"`
	}
	if err := s.parseBody(r, &body); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := s.storage.AddComment(ctx, vars["id"], actor, body.Text); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, map[string]string{"message": "comment added"}, "comment_add")
}

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	events, err := s.storage.GetEvents(ctx, vars["id"], 100)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, events, "comment_list")
}

func (s *Server) handleAddLabel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := s.getActor(r)
	vars := mux.Vars(r)

	var body struct {
		Label string `json:"label"`
	}
	if err := s.parseBody(r, &body); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := s.storage.AddLabel(ctx, vars["id"], body.Label, actor); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, map[string]string{"message": "label added"}, "label_add")
}

func (s *Server) handleRemoveLabel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := s.getActor(r)
	vars := mux.Vars(r)

	if err := s.storage.RemoveLabel(ctx, vars["id"], vars["label"], actor); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, map[string]string{"message": "label removed"}, "label_remove")
}

func (s *Server) handleAddDependency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := s.getActor(r)
	vars := mux.Vars(r)

	var body struct {
		DependsOn string `json:"depends_on"`
		Type      string `json:"type"`
	}
	if err := s.parseBody(r, &body); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err)
		return
	}

	depType := types.DepBlocks
	if body.Type != "" {
		depType = types.DependencyType(body.Type)
	}

	dep := &types.Dependency{
		IssueID:     vars["id"],
		DependsOnID: body.DependsOn,
		Type:        depType,
	}

	if err := s.storage.AddDependency(ctx, dep, actor); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, map[string]string{"message": "dependency added"}, "dep_add")
}

func (s *Server) handleRemoveDependency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := s.getActor(r)
	vars := mux.Vars(r)

	if err := s.storage.RemoveDependency(ctx, vars["id"], vars["depId"], actor); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, map[string]string{"message": "dependency removed"}, "dep_remove")
}

func (s *Server) handleDependencyTree(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	query := r.URL.Query()

	maxDepth := 10
	if d := query.Get("max_depth"); d != "" {
		maxDepth, _ = strconv.Atoi(d)
	}

	tree, err := s.storage.GetDependencyTree(ctx, vars["id"], maxDepth, false, false)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, tree, rpc.OpDepTree)
}

func (s *Server) handleEpicStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	epics, err := s.storage.GetEpicsEligibleForClosure(ctx)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.writeSuccess(w, r, epics, rpc.OpEpicStatus)
}

// Placeholder stubs for remaining endpoints
func (s *Server) handleCompact(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, r, http.StatusNotImplemented, fmt.Errorf("not implemented"))
}

func (s *Server) handleCompactStats(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, r, http.StatusNotImplemented, fmt.Errorf("not implemented"))
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, r, http.StatusNotImplemented, fmt.Errorf("not implemented"))
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, r, http.StatusNotImplemented, fmt.Errorf("not implemented"))
}

func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, r, http.StatusNotImplemented, fmt.Errorf("not implemented"))
}

// handleGetConfig handles GET /config/{key}
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	value, err := s.storage.GetConfig(ctx, vars["key"])
	if err != nil {
		s.writeError(w, r, http.StatusNotFound, err)
		return
	}

	result := map[string]string{
		"key":   vars["key"],
		"value": value,
	}
	s.writeSuccess(w, r, result, "config_get")
}

// handleSetConfig handles PUT /config/{key}
func (s *Server) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	var body struct {
		Value string `json:"value"`
	}
	if err := s.parseBody(r, &body); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := s.storage.SetConfig(ctx, vars["key"], body.Value); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	result := map[string]string{
		"key":   vars["key"],
		"value": body.Value,
	}
	s.writeSuccess(w, r, result, "config_set")
}
