package postgres

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RunMigrations applies all *.up.sql migration scripts found in migrationDir in alphanumeric order.
func RunMigrations(db *DB, migrationDir string) error {
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return fmt.Errorf("failed to read migration directory %s: %w", migrationDir, err)
	}

	var upFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			upFiles = append(upFiles, entry.Name())
		}
	}

	sort.Strings(upFiles)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, file := range upFiles {
		filePath := filepath.Join(migrationDir, file)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		log.Printf("Applying migration: %s", file)
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("failed executing migration %s: %w", file, err)
		}
	}

	return nil
}
