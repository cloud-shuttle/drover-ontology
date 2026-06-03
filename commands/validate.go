package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Run full schema and Drover policy checks across the graph",
	Run: func(cmd *cobra.Command, args []string) {
		ont := getOntology()
		if ont == nil {
			os.Exit(1)
		}

		strict, _ := cmd.Flags().GetBool("strict")
		typFlag, _ := cmd.Flags().GetString("type")

		errs := ont.Validate()

		var filteredErrs []string
		for _, e := range errs {
			msg := e.Error()
			if typFlag != "" {
				// Only include errors relevant to the requested type (case-insensitive match)
				if !strings.Contains(strings.ToLower(msg), strings.ToLower(typFlag)) {
					continue
				}
			}
			filteredErrs = append(filteredErrs, msg)
		}

		if len(filteredErrs) == 0 {
			fmt.Println("✅ Validation successful! Graph satisfies all active schema and Drover policies.")
			return
		}

		fmt.Fprintf(os.Stderr, "❌ Validation failed! Found %d policy violations:\n\n", len(filteredErrs))
		for i, msg := range filteredErrs {
			fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, msg)
		}

		if strict {
			os.Exit(1)
		}
	},
}

func init() {
	validateCmd.Flags().Bool("strict", false, "Exit with status 1 if any policy violation is found")
	validateCmd.Flags().String("type", "", "Only validate and report violations for a specific entity type")
}
