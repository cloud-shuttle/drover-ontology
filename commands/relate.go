package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var relateCmd = &cobra.Command{
	Use:   "relate [from] [relation] [to]",
	Short: "Create a relationship between two entities",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		from, rel, to := args[0], args[1], args[2]
		propsStr, _ := cmd.Flags().GetStringToString("prop")
		props := propsFromStringMap(propsStr)

		ont := getOntology()
		if ont == nil {
			return
		}

		if err := ont.Relate(from, rel, to, props); err != nil {
			fmt.Printf("❌ %v\n", err)
			return
		}

		fmt.Printf("✅ Related %s --[%s]--> %s\n", from, rel, to)
	},
}

func init() {
	relateCmd.Flags().StringToStringP("prop", "p", nil, "Optional relationship properties")
}
