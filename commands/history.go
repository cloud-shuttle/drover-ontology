package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/drover-org/drover-ontology/internal/ontology"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history [entity-id]",
	Short: "Show the mutation history for the entire graph or a specific entity",
	Run: func(cmd *cobra.Command, args []string) {
		logReader, err := ontology.NewLog(graphPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to initialize log reader at %s: %v\n", graphPath, err)
			os.Exit(1)
		}

		_, ops, err := logReader.LoadAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to read log: %v\n", err)
			os.Exit(1)
		}

		var targetID string
		if len(args) > 0 {
			targetID = args[0]
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")

		var filteredOps []ontology.Operation
		for _, op := range ops {
			if targetID != "" {
				// Filter operations affecting the requested entity ID:
				//   - op.ID matches targetID
				//   - op.From matches targetID
				//   - op.To matches targetID
				//   - op.Entity.ID matches targetID
				matches := false
				if op.ID == targetID {
					matches = true
				} else if op.From == targetID {
					matches = true
				} else if op.To == targetID {
					matches = true
				} else if op.Entity != nil && op.Entity.ID == targetID {
					matches = true
				}
				if !matches {
					continue
				}
			}
			filteredOps = append(filteredOps, op)
		}

		if len(filteredOps) == 0 {
			if targetID != "" {
				fmt.Printf("No history found for entity: %s\n", targetID)
			} else {
				fmt.Println("No history found in the audit log.")
			}
			return
		}

		if jsonFlag {
			for _, op := range filteredOps {
				b, err := json.Marshal(op)
				if err == nil {
					fmt.Println(string(b))
				}
			}
			return
		}

		// Pretty chronological output
		if targetID != "" {
			fmt.Printf("Timeline for Entity: %s\n\n", targetID)
		} else {
			fmt.Print("Global Context Graph Timeline\n\n")
		}

		for _, op := range filteredOps {
			tsStr := op.Ts.UTC().Format("2006-01-02 15:04:05 MST")
			sourceStr := op.Source
			if sourceStr == "" {
				sourceStr = "Agent"
			}

			switch op.Op {
			case "create":
				if op.Entity != nil {
					fmt.Printf("[%s] CREATE by %s\n", tsStr, sourceStr)
					fmt.Printf("  Type: %s\n", op.Entity.Type)
					if len(op.Entity.Properties) > 0 {
						fmt.Println("  Properties:")
						for k, v := range op.Entity.Properties {
							fmt.Printf("    - %s: %v\n", k, v)
						}
					}
				} else {
					fmt.Printf("[%s] CREATE (empty entity) by %s\n", tsStr, sourceStr)
				}
			case "update":
				fmt.Printf("[%s] UPDATE by %s\n", tsStr, sourceStr)
				if len(op.Set) > 0 {
					fmt.Println("  Set:")
					for k, v := range op.Set {
						fmt.Printf("    - %s: %v\n", k, v)
					}
				}
			case "relate":
				fmt.Printf("[%s] RELATED to %s under %q by %s\n", tsStr, op.To, op.Rel, sourceStr)
				if op.From != targetID {
					fmt.Printf("  From: %s\n", op.From)
				}
				if len(op.Properties) > 0 {
					fmt.Println("  Properties:")
					for k, v := range op.Properties {
						fmt.Printf("    - %s: %v\n", k, v)
					}
				}
			case "unrelate":
				fmt.Printf("[%s] UNRELATED from %s under %q by %s\n", tsStr, op.To, op.Rel, sourceStr)
				if op.From != targetID {
					fmt.Printf("  From: %s\n", op.From)
				}
			case "invalidate":
				fmt.Printf("[%s] INVALIDATED by %s\n", tsStr, sourceStr)
				fmt.Printf("  ValidUntil: %s\n", op.ValidUntil)
			case "tombstone":
				fmt.Printf("[%s] TOMBSTONED by %s\n", tsStr, sourceStr)
				if op.Reason != "" {
					fmt.Printf("  Reason: %s\n", op.Reason)
				}
			default:
				fmt.Printf("[%s] %s by %s\n", tsStr, strings.ToUpper(op.Op), sourceStr)
			}
			fmt.Println()
		}
	},
}

func init() {
	historyCmd.Flags().Bool("json", false, "Output raw JSON objects (useful for automated agents)")
}
