package commands

import (
	"fmt"
	"os"

	"github.com/drover-org/drover-ontology/internal/ontology"
	"github.com/spf13/cobra"
)

var embedCmd = &cobra.Command{
	Use:   "embed-model",
	Short: "Manage embedding models for hybrid search",
}

var embedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available and current embedding model",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Current embedding model: %s\n\n", ontology.GetCurrentModel())

		models, err := ontology.ListEmbeddingModels()
		if err != nil {
			fmt.Println("Could not list models from Ollama:", err)
			fmt.Println("Make sure Ollama is running: ollama serve")
			return
		}

		fmt.Println("Available models in Ollama:")
		for _, m := range models {
			fmt.Printf("  - %s (size: %d, modified: %s)\n", m.Name, m.Size, m.Modified)
		}
	},
}

var embedSetCmd = &cobra.Command{
	Use:   "set <model>",
	Short: "Set the active embedding model",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		model := args[0]
		if err := ontology.SetCurrentModel(model); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set model: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Embedding model set to: %s\n", model)
	},
}

var embedPullCmd = &cobra.Command{
	Use:   "pull <model>",
	Short: "Pull an embedding model from Ollama registry",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		model := args[0]
		fmt.Printf("Pulling model %s via Ollama...\n", model)
		if err := ontology.PullModel(model); err != nil {
			fmt.Fprintf(os.Stderr, "Pull failed: %v\n", err)
			fmt.Println("You can still use dummy embeddings as fallback.")
			return
		}
		fmt.Printf("Successfully pulled %s\n", model)
	},
}

var embedBatchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Batch embed all entities missing embeddings (uses current model)",
	Run: func(cmd *cobra.Command, args []string) {
		ont := getOntology()
		if ont == nil {
			return
		}

		fmt.Printf("Starting batch embedding using model: %s\n", ontology.GetCurrentModel())
		count, err := ont.BatchEmbed(0, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Batch embedding failed: %v\n", err)
			return
		}
		fmt.Printf("✅ Batch embedding complete. Generated embeddings for %d new entities.\n", count)
	},
}

func init() {
	embedBatchCmd.Flags().Int("limit", 0, "Limit number of entities to process (0 = all)")

	embedCmd.AddCommand(embedListCmd)
	embedCmd.AddCommand(embedSetCmd)
	embedCmd.AddCommand(embedPullCmd)
	embedCmd.AddCommand(embedBatchCmd)
}
