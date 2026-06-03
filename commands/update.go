package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [id]",
	Short: "Update properties on an entity",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		propsStr, _ := cmd.Flags().GetStringToString("prop")

		if len(propsStr) == 0 {
			fmt.Println("No properties provided. Use --prop key=value")
			return
		}

		props := propsFromStringMap(propsStr)

		ont := getOntology()
		if ont == nil {
			return
		}

		if err := ont.Update(id, props); err != nil {
			fmt.Printf("❌ %v\n", err)
			return
		}

		fmt.Printf("✅ Updated %s\n", id)
	},
}

func init() {
	updateCmd.Flags().StringToStringP("prop", "p", nil, "Properties to update (key=value)")
}
