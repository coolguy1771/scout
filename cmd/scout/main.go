package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "scout",
	Short: "Scout - CRE Site-Selection Platform",
	Long: `Scout is a geospatial platform for Commercial Real Estate professionals 
to identify and rank developable parcels by overlaying constraints and infrastructure proximity.`,
	Version: "0.1.0",
}

func init() {
	rootCmd.AddCommand(apiCmd)
	rootCmd.AddCommand(workerCmd)
	rootCmd.AddCommand(tokenCmd)
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(seedCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
