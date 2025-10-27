package autoimport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// Notifier handles user notifications during import
type Notifier interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// stderrNotifier implements Notifier using stderr
type stderrNotifier struct {
	debug bool
}

func (n *stderrNotifier) Debugf(format string, args ...interface{}) {
	if n.debug {
		fmt.Fprintf(os.Stderr, "Debug: "+format+"\n", args...)
	}
}

func (n *stderrNotifier) Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func (n *stderrNotifier) Warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}

func (n *stderrNotifier) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

// NewStderrNotifier creates a notifier that writes to stderr
func NewStderrNotifier(debug bool) Notifier {
	return &stderrNotifier{debug: debug}
}

// ImportFunc is called to perform the actual import after detecting staleness
// It receives the parsed issues and should return created/updated counts and ID mappings
// The ID mapping maps old IDs -> new IDs for collision resolution
type ImportFunc func(ctx context.Context, issues []*types.Issue) (created, updated int, idMapping map[string]string, err error)

// AutoImportIfNewer checks if JSONL is newer than last import and imports if needed
// dbPath is the full path to the database file (e.g., /path/to/.beads/bd.db)
func AutoImportIfNewer(ctx context.Context, store storage.Storage, dbPath string, notify Notifier, importFunc ImportFunc, onChanged func(needsFullExport bool)) error {
	if notify == nil {
		notify = NewStderrNotifier(os.Getenv("BD_DEBUG") != "")
	}

	// Find JSONL using database directory (same logic as beads.FindJSONLPath)
	dbDir := filepath.Dir(dbPath)
	pattern := filepath.Join(dbDir, "*.jsonl")
	matches, err := filepath.Glob(pattern)
	var jsonlPath string
	if err == nil && len(matches) > 0 {
		jsonlPath = matches[0]
	} else {
		jsonlPath = filepath.Join(dbDir, "issues.jsonl")
	}
	if jsonlPath == "" {
		notify.Debugf("auto-import skipped, JSONL not found")
		return nil
	}

	jsonlData, err := os.ReadFile(jsonlPath) // #nosec G304 - controlled path from config
	if err != nil {
		notify.Debugf("auto-import skipped, JSONL not readable: %v", err)
		return nil
	}

	hasher := sha256.New()
	hasher.Write(jsonlData)
	currentHash := hex.EncodeToString(hasher.Sum(nil))

	lastHash, err := store.GetMetadata(ctx, "last_import_hash")
	if err != nil {
		notify.Debugf("metadata read failed (%v), treating as first import", err)
		lastHash = ""
	}

	if currentHash == lastHash {
		notify.Debugf("auto-import skipped, JSONL unchanged (hash match)")
		return nil
	}

	notify.Debugf("auto-import triggered (hash changed)")

	if err := checkForMergeConflicts(jsonlData, jsonlPath); err != nil {
		notify.Errorf("%v", err)
		return err
	}

	allIssues, err := parseJSONL(jsonlData, notify)
	if err != nil {
		notify.Errorf("Auto-import skipped: %v", err)
		return err
	}

	created, updated, idMapping, err := importFunc(ctx, allIssues)
	if err != nil {
		notify.Errorf("Auto-import failed: %v", err)
		return err
	}

	// Show detailed remapping if any
	showRemapping(allIssues, idMapping, notify)

	changed := (created + updated + len(idMapping)) > 0
	if changed && onChanged != nil {
		needsFullExport := len(idMapping) > 0
		onChanged(needsFullExport)
	}

	if err := store.SetMetadata(ctx, "last_import_hash", currentHash); err != nil {
		notify.Warnf("failed to update last_import_hash after import: %v", err)
		notify.Warnf("This may cause auto-import to retry the same import on next operation.")
	}

	importTime := time.Now().Format(time.RFC3339)
	if err := store.SetMetadata(ctx, "last_import_time", importTime); err != nil {
		notify.Warnf("failed to update last_import_time after import: %v", err)
	}

	return nil
}

// showRemapping displays ID remapping details
func showRemapping(allIssues []*types.Issue, idMapping map[string]string, notify Notifier) {
	if len(idMapping) == 0 {
		return
	}

	// Build title lookup map
	titleByID := make(map[string]string)
	for _, issue := range allIssues {
		titleByID[issue.ID] = issue.Title
	}

	// Sort by old ID for consistent output
	type mapping struct {
		oldID string
		newID string
	}
	mappings := make([]mapping, 0, len(idMapping))
	for oldID, newID := range idMapping {
		mappings = append(mappings, mapping{oldID, newID})
	}
	
	// Sort by old ID
	for i := 0; i < len(mappings); i++ {
		for j := i + 1; j < len(mappings); j++ {
			if mappings[i].oldID > mappings[j].oldID {
				mappings[i], mappings[j] = mappings[j], mappings[i]
			}
		}
	}

	maxShow := 10
	numRemapped := len(mappings)
	if numRemapped < maxShow {
		maxShow = numRemapped
	}

	notify.Infof("\nAuto-import: remapped %d colliding issue(s) to new IDs:", numRemapped)
	for i := 0; i < maxShow; i++ {
		m := mappings[i]
		title := titleByID[m.oldID]
		notify.Infof("  %s → %s (%s)", m.oldID, m.newID, title)
	}
	if numRemapped > maxShow {
		notify.Infof("  ... and %d more", numRemapped-maxShow)
	}
	notify.Infof("")
}

func checkForMergeConflicts(jsonlData []byte, jsonlPath string) error {
	lines := bytes.Split(jsonlData, []byte("\n"))
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("<<<<<<< ")) ||
			bytes.Equal(trimmed, []byte("=======")) ||
			bytes.HasPrefix(trimmed, []byte(">>>>>>> ")) {
			return fmt.Errorf("❌ Git merge conflict detected in %s\n\n"+
				"The JSONL file contains unresolved merge conflict markers.\n"+
				"This prevents auto-import from loading your issues.\n\n"+
				"To resolve:\n"+
				"  1. Resolve the merge conflict in your Git client, OR\n"+
				"  2. Export from database to regenerate clean JSONL:\n"+
				"     bd export -o %s\n\n"+
				"After resolving, commit the fixed JSONL file.\n", jsonlPath, jsonlPath)
		}
	}
	return nil
}

func parseJSONL(jsonlData []byte, notify Notifier) ([]*types.Issue, error) {
	scanner := bufio.NewScanner(bytes.NewReader(jsonlData))
	scanner.Buffer(make([]byte, 0, 1024), 2*1024*1024)
	var allIssues []*types.Issue
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if line == "" {
			continue
		}

		var issue types.Issue
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			snippet := line
			if len(snippet) > 80 {
				snippet = snippet[:80] + "..."
			}
			return nil, fmt.Errorf("parse error at line %d: %v\nSnippet: %s", lineNo, err, snippet)
		}

		if issue.Status == types.StatusClosed && issue.ClosedAt == nil {
			now := time.Now()
			issue.ClosedAt = &now
		}

		allIssues = append(allIssues, &issue)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return allIssues, nil
}

// CheckStaleness checks if JSONL is newer than last import
// dbPath is the full path to the database file
func CheckStaleness(ctx context.Context, store storage.Storage, dbPath string) (bool, error) {
	lastImportStr, err := store.GetMetadata(ctx, "last_import_time")
	if err != nil {
		return false, nil
	}
	
	lastImportTime, err := time.Parse(time.RFC3339, lastImportStr)
	if err != nil {
		return false, nil
	}
	
	// Find JSONL using database directory
	dbDir := filepath.Dir(dbPath)
	pattern := filepath.Join(dbDir, "*.jsonl")
	matches, err := filepath.Glob(pattern)
	var jsonlPath string
	if err == nil && len(matches) > 0 {
		jsonlPath = matches[0]
	} else {
		jsonlPath = filepath.Join(dbDir, "issues.jsonl")
	}
	
	if jsonlPath == "" {
		return false, nil
	}
	
	stat, err := os.Stat(jsonlPath)
	if err != nil {
		return false, nil
	}
	
	return stat.ModTime().After(lastImportTime), nil
}
