package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tracelyn/internal/recorder"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop recording terminal session",
	Long:  `Stops the current recording session and saves the session file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Stopping recording session...")
		return recorder.StopRecording()
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
