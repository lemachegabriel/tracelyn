package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tracelyn/internal/recorder"
)

var mergeCmd = &cobra.Command{
	Use:   "merge <session-id-1> <session-id-2> [session-id-N...] -o <output-file>",
	Short: "Merge multiple session files into one",
	Long:  `Merges multiple recording sessions into a single file, sorted chronologically.`,
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputFile, _ := cmd.Flags().GetString("output")
		if outputFile == "" {
			return fmt.Errorf("output file is required (use -o flag)")
		}

		if err := recorder.MergeSessions(args, outputFile); err != nil {
			return err
		}

		fmt.Printf("Successfully merged %d sessions into: %s\n", len(args), outputFile)
		return nil
	},
}

func init() {
	mergeCmd.Flags().StringP("output", "o", "", "Output file path (required)")
	mergeCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(mergeCmd)
}
