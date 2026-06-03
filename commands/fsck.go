package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var fsckCmd = &cobra.Command{
	Use:   "fsck",
	Short: "Run integrity checks on the ontology store",
	Long: `Run integrity checks comparing the current state (from SQLite or in-memory)
against a full replay of the append-only JSONL audit log.

This detects:
- Corruption or drift between the durable JSONL log and materialized state
- Violations of Drover domain policies (e.g. approved Plan without Review,
  Term with status=accepted but no governed_by link)
- Temporal validity problems

Recommended before important operations or on a schedule in production.`,
	Run: func(cmd *cobra.Command, args []string) {
		ont := getOntology()
		if ont == nil {
			os.Exit(1)
		}

		checkInvalidateOnly, _ := cmd.Flags().GetBool("check-invalidate")

		fmt.Println("Running ontology integrity check (replay + policies)...")
		if err := ont.IntegrityCheck(); err != nil {
			fmt.Printf("❌ Integrity check failed: %v\n", err)
			os.Exit(1)
		}

		if checkInvalidateOnly {
			fmt.Println("✅ Invalidate / temporal validity checks passed")
			fmt.Println("   (Note: --check-invalidate currently runs the full integrity + policy check)")
		} else {
			fmt.Println("✅ Full integrity check passed (replay parity + policies)")
		}
	},
}

func init() {
	fsckCmd.Flags().Bool("check-invalidate", false, "Run focused checks on invalidate/temporal validity (currently runs full integrity check)")
}
