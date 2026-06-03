package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Hybrid vector + graph search across the ontology",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := args[0]
		limit, _ := cmd.Flags().GetInt("limit")
		alpha, _ := cmd.Flags().GetFloat64("alpha")

		ont := getOntology()
		if ont == nil {
			return
		}
		results, err := ont.HybridSearch(query, limit, alpha)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Search failed: %v\n", err)
			os.Exit(1)
		}

		if len(results) == 0 {
			fmt.Println("No results found.")
			return
		}

		fmt.Printf("Found %d results for query: %q\n\n", len(results), query)

		for i, r := range results {
			props, _ := json.MarshalIndent(r.Properties, "    ", "  ")
			fmt.Printf("%d. %s (%s)  score=%.4f\n", i+1, r.ID, r.Type, r.Score)
			fmt.Printf("    %s\n\n", string(props))
		}
	},
}

func init() {
	searchCmd.Flags().Int("limit", 10, "Maximum number of results")
	searchCmd.Flags().Float64("alpha", 0.7, "Weight between vector similarity and graph boost (0.0 - 1.0)")
}
