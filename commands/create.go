package commands

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create [type] [id]",
	Short: "Create a new entity with temporal validity",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		typ := args[0]
		id := ""
		if len(args) > 1 {
			id = args[1]
		} else {
			id = fmt.Sprintf("%s_%s", typ[:min(4, len(typ))], uuid.New().String()[:8])
		}

		propsStr, _ := cmd.Flags().GetStringToString("prop")
		propsIface := propsFromStringMap(propsStr)

		ont := getOntology()
		if ont == nil {
			return
		}

		if err := ont.CreateEntity(typ, id, propsIface); err != nil {
			fmt.Printf("❌ Failed to create: %v\n", err)
			return
		}

		fmt.Printf("✅ Created %s (%s)\n", id, typ)
	},
}

func init() {
	createCmd.Flags().StringToStringP("prop", "p", nil, "Initial properties (key=value)")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

