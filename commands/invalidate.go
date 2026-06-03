package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var invalidateCmd = &cobra.Command{
	Use:   "invalidate [id]",
	Short: "Temporally invalidate an entity (soft delete with valid_until)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		until, _ := cmd.Flags().GetString("until")

		ont := getOntology()
		if ont == nil {
			return
		}

		if err := ont.Invalidate(id, until); err != nil {
			fmt.Printf("❌ %v\n", err)
			return
		}

		// Note: if until was empty, Invalidate defaulted to now inside
		if until == "" {
			until = time.Now().UTC().Format(time.RFC3339)
		}
		fmt.Printf("✅ Invalidated %s (valid until %s)\n", id, until)
	},
}

func init() {
	invalidateCmd.Flags().String("until", "", "ISO8601 timestamp (default = now)")
}
