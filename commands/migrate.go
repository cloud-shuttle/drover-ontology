package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate from JSONL audit log to SQLite backend (with temporal fields)",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting migration from JSONL to SQLite...")

		ont := getOntology()
		if ont == nil {
			return
		}

		count, err := ont.Migrate()
		if err != nil {
			fmt.Printf("❌ Migration failed: %v\n", err)
			return
		}

		fmt.Printf("✅ Migration complete! %d entities migrated to SQLite.\n", count)
		fmt.Println("   The SQLite database is now the primary store.")
	},
}

