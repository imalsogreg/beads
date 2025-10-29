package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/imalsogreg/beads/internal/types"
)

func BenchmarkGetTier1Candidates(b *testing.B) {
	store, cleanup := setupBenchDB(b)
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		issue := &types.Issue{
			ID:          generateID(b, "bd-", i),
			Title:       "Benchmark issue",
			Description: "Test description for benchmarking",
			Status:      "closed",
			Priority:    2,
			IssueType:   "task",
			ClosedAt:    timePtr(time.Now().Add(-40 * 24 * time.Hour)),
		}
		if err := store.CreateIssue(ctx, issue, "test"); err != nil {
			b.Fatalf("Failed to create issue: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetTier1Candidates(ctx)
		if err != nil {
			b.Fatalf("GetTier1Candidates failed: %v", err)
		}
	}
}

func BenchmarkGetTier2Candidates(b *testing.B) {
	store, cleanup := setupBenchDB(b)
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		issue := &types.Issue{
			ID:          generateID(b, "bd-", i),
			Title:       "Benchmark issue",
			Description: "Test",
			Status:      "closed",
			Priority:    2,
			IssueType:   "task",
			ClosedAt:    timePtr(time.Now().Add(-100 * 24 * time.Hour)),
		}
		if err := store.CreateIssue(ctx, issue, "test"); err != nil {
			b.Fatalf("Failed to create issue: %v", err)
		}

		_, err := store.db.ExecContext(ctx, `
			UPDATE issues 
			SET compaction_level = 1, 
			    compacted_at = datetime('now', '-95 days'),
			    original_size = 1000
			WHERE id = ?
		`, issue.ID)
		if err != nil {
			b.Fatalf("Failed to set compaction level: %v", err)
		}

		for j := 0; j < 120; j++ {
			if err := store.AddComment(ctx, issue.ID, "test", "comment"); err != nil {
				b.Fatalf("Failed to add event: %v", err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetTier2Candidates(ctx)
		if err != nil {
			b.Fatalf("GetTier2Candidates failed: %v", err)
		}
	}
}

func BenchmarkCheckEligibility(b *testing.B) {
	store, cleanup := setupBenchDB(b)
	defer cleanup()
	ctx := context.Background()

	issue := &types.Issue{
		ID:          "bd-1",
		Title:       "Eligible",
		Description: "Test",
		Status:      "closed",
		Priority:    2,
		IssueType:   "task",
		ClosedAt:    timePtr(time.Now().Add(-40 * 24 * time.Hour)),
	}
	if err := store.CreateIssue(ctx, issue, "test"); err != nil {
		b.Fatalf("Failed to create issue: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := store.CheckEligibility(ctx, issue.ID, 1)
		if err != nil {
			b.Fatalf("CheckEligibility failed: %v", err)
		}
	}
}

func generateID(b testing.TB, prefix string, n int) string{
	b.Helper()
	return prefix + string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func setupBenchDB(tb testing.TB) (*SQLiteStorage, func()) {
	tb.Helper()
	tmpDB := tb.TempDir() + "/test.db"
	store, err := New(tmpDB)
	if err != nil {
		tb.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	if err := store.SetConfig(ctx, "compact_tier1_days", "30"); err != nil {
		tb.Fatalf("Failed to set config: %v", err)
	}
	if err := store.SetConfig(ctx, "compact_tier1_dep_levels", "2"); err != nil {
		tb.Fatalf("Failed to set config: %v", err)
	}

	return store, func() {
		store.Close()
	}
}
