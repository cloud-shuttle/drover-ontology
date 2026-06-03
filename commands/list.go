package commands

import (
	"fmt"

	"github.com/drover-org/drover-ontology/internal/ontology"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list [type]",
	Short: "List entities (optionally filtered by type)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filter := ""
		if len(args) > 0 {
			filter = args[0]
		}

		ont := getOntology()
		if ont == nil {
			return
		}

		entities := ont.All()
		count := 0
		for _, e := range entities {
			if ontology.IsDeleted(e) {
				continue
			}
			if filter == "" || e.Type == filter {
				fmt.Printf("%-20s %s\n", e.ID, e.Type)
				count++
			}
		}
		fmt.Printf("\nTotal: %d entities\n", count)
	},
}

