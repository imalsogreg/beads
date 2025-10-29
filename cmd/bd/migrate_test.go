package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/imalsogreg/beads/internal/storage/sqlite"
)

func TestMigrateCommand(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0750); err != nil {
		t.Fatalf("Failed to create .beads directory: %v", err)
	}

	t.Run("no databases", func(t *testing.T) {
		databases, err := detectDatabases(beadsDir)
		if err != nil {
			t.Fatalf("detectDatabases failed: %v", err)
		}
		if len(databases) != 0 {
			t.Errorf("Expected 0 databases, got %d", len(databases))
		}
	})

	t.Run("single old database", func(t *testing.T) {
		// Create old database
		oldDBPath := filepath.Join(beadsDir, "vc.db")
		store, err := sqlite.New(oldDBPath)
		if err != nil {
			t.Fatalf("Failed to create old database: %v", err)
		}

		// Set old version
		ctx := context.Background()
		if err := store.SetMetadata(ctx, "bd_version", "0.16.0"); err != nil {
			t.Fatalf("Failed to set old version: %v", err)
		}
		_ = store.Close()

		// Detect databases
		databases, err := detectDatabases(beadsDir)
		if err != nil {
			t.Fatalf("detectDatabases failed: %v", err)
		}
		if len(databases) != 1 {
			t.Fatalf("Expected 1 database, got %d", len(databases))
		}
		if databases[0].version != "0.16.0" {
			t.Errorf("Expected version 0.16.0, got %s", databases[0].version)
		}

		// Migrate to beads.db
		targetPath := filepath.Join(beadsDir, "beads.db")
		if err := os.Rename(oldDBPath, targetPath); err != nil {
			t.Fatalf("Failed to migrate database: %v", err)
		}

		// Verify migration
		databases, err = detectDatabases(beadsDir)
		if err != nil {
			t.Fatalf("detectDatabases failed after migration: %v", err)
		}
		if len(databases) != 1 {
			t.Fatalf("Expected 1 database after migration, got %d", len(databases))
		}
		if filepath.Base(databases[0].path) != "beads.db" {
			t.Errorf("Expected beads.db, got %s", filepath.Base(databases[0].path))
		}
	})

	t.Run("version detection", func(t *testing.T) {
		// Test getDBVersion with beads.db from previous test
		dbPath := filepath.Join(beadsDir, "beads.db")
		version := getDBVersion(dbPath)
		if version != "0.16.0" {
			t.Errorf("Expected version 0.16.0, got %s", version)
		}

		// Update version
		store, err := sqlite.New(dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}
		ctx := context.Background()
		if err := store.SetMetadata(ctx, "bd_version", Version); err != nil {
			t.Fatalf("Failed to update version: %v", err)
		}
		_ = store.Close()

		// Verify updated version
		version = getDBVersion(dbPath)
		if version != Version {
			t.Errorf("Expected version %s, got %s", Version, version)
		}
	})
}

func TestFormatDBList(t *testing.T) {
	dbs := []*dbInfo{
		{path: "/tmp/.beads/beads.db", version: "0.17.5"},
		{path: "/tmp/.beads/old.db", version: "0.16.0"},
	}

	result := formatDBList(dbs)
	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(result))
	}

	if result[0]["name"] != "beads.db" {
		t.Errorf("Expected name beads.db, got %s", result[0]["name"])
	}
	if result[0]["version"] != "0.17.5" {
		t.Errorf("Expected version 0.17.5, got %s", result[0]["version"])
	}

	if result[1]["name"] != "old.db" {
		t.Errorf("Expected name old.db, got %s", result[1]["name"])
	}
	if result[1]["version"] != "0.16.0" {
		t.Errorf("Expected version 0.16.0, got %s", result[1]["version"])
	}
}
