package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Soft-delete an entity (tombstone + invalidate for audit + visibility)",
	Long: `Permanently removes an entity from default views (list, get, search) while
preserving a complete audit trail in the append-only JSONL log.

The delete operation performs two steps:
  1. tombstone  - records the deletion with an optional reason (status=tombstoned)
  2. invalidate - sets ValidUntil=now so the entity is hidden from default queries

History remains queryable via the log (or future history/audit commands).

This is the recommended way to remove entities. Hard deletes are not supported.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		reason, _ := cmd.Flags().GetString("reason")

		ont := getOntology()
		if ont == nil {
			return
		}

		// Check existence for better UX (the ops themselves would silently no-op)
		if _, exists := ont.Get(id); !exists {
			fmt.Printf("❌ Entity not found: %s\n", id)
			return
		}

		if err := ont.SoftDelete(id, reason); err != nil {
			fmt.Printf("❌ %v\n", err)
			return
		}

		if reason != "" {
			fmt.Printf("✅ Deleted %s (tombstoned + invalidated, reason: %s)\n", id, reason)
		} else {
			fmt.Printf("✅ Deleted %s (tombstoned + invalidated)\n", id)
		}
	},
}

func init() {
	deleteCmd.Flags().StringP("reason", "r", "", "Optional reason for the deletion (recorded in the tombstone operation)")
}
