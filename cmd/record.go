package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tracelyn/internal/recorder"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Start recording terminal session",
	Long:  `Starts recording all terminal commands and outputs to a session file in the sessions/ directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Starting new recording session...")
		return recorder.StartRecording()
	},
}

func init() {
	rootCmd.AddCommand(recordCmd)
}
