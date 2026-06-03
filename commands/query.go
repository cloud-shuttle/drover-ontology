package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query entities with simple filters",
	Run: func(cmd *cobra.Command, args []string) {
		typ, _ := cmd.Flags().GetString("type")
		status, _ := cmd.Flags().GetString("status")

		ont := getOntology()
		if ont == nil {
			return
		}

		entities := ont.All()
		count := 0
		for _, e := range entities {
			match := true
			if typ != "" && e.Type != typ {
				match = false
			}
			if status != "" {
				if s, ok := e.Properties["status"].(string); !ok || s != status {
					match = false
				}
			}
			if match {
				fmt.Printf("%s  %s\n", e.ID, e.Type)
				count++
			}
		}
		fmt.Printf("Total: %d entities\n", count)
	},
}

func init() {
	queryCmd.Flags().String("type", "", "Filter by entity type")
	queryCmd.Flags().String("status", "", "Filter by status property")
}
