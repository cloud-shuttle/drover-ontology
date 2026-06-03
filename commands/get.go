package commands

import (
	"encoding/json"
	"fmt"

	"github.com/drover-org/drover-ontology/internal/ontology"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get full details of an entity",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]

		ont := getOntology()
		if ont == nil {
			return
		}

		entities := ont.All()

		e, ok := entities[id]
		if !ok || ontology.IsDeleted(e) {
			fmt.Printf("❌ Entity not found: %s\n", id)
			return
		}

		fmt.Printf("ID:          %s\n", e.ID)
		fmt.Printf("Type:        %s\n", e.Type)
		fmt.Printf("Created:     %s\n", e.Created)
		fmt.Printf("Updated:     %s\n", e.Updated)
		fmt.Printf("Valid From:  %s\n", e.ValidFrom)
		if e.ValidUntil != nil {
			fmt.Printf("Valid Until: %s\n", e.ValidUntil.Format("2006-01-02T15:04:05Z"))
		}

		fmt.Println("Properties:")
		b, _ := json.MarshalIndent(e.Properties, "  ", "  ")
		fmt.Println(" ", string(b))
	},
}

