// backfill-skills populates the skill_sizes table for all skills that have been
// invoked but whose file size was not recorded at scan time.
//
// Usage:
//
//	go run ./cmd/backfill-skills
//	TOKENTALLY_DB=/path/to/tokentally.db go run ./cmd/backfill-skills
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"tokentally/internal/db"
	"tokentally/internal/skills"
)

func main() {
	dbPath := os.Getenv("TOKENTALLY_DB")
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("home dir: %v", err)
		}
		dbPath = filepath.Join(home, ".claude", "tokentally.db")
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	rows, err := conn.Query(
		`SELECT DISTINCT target FROM tool_calls
		 WHERE tool_name='Skill' AND target != '' AND target IS NOT NULL
		 ORDER BY target`,
	)
	if err != nil {
		log.Fatalf("query skills: %v", err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			names = append(names, n)
		}
	}
	rows.Close()

	fmt.Printf("Found %d distinct skills\n\n", len(names))

	updated, skipped := 0, 0
	for _, name := range names {
		b, ok := skills.Bytes(name)
		if !ok {
			fmt.Printf("  skip  %s\n", name)
			skipped++
			continue
		}
		if err := db.UpsertSkillSize(conn, name, b); err != nil {
			fmt.Printf("  error %-45s  %v\n", name, err)
			continue
		}
		fmt.Printf("  ok    %-45s  %6d bytes  ~%d tokens\n", name, b, b/4)
		updated++
	}

	fmt.Printf("\nDone: %d updated, %d skipped (file not found)\n", updated, skipped)
}
