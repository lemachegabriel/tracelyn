package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tracelyn/internal/recorder"
)

var stopCmd = &cobra.Command{
	Use:   "stop [session-id]",
	Short: "Stop recording session",
	Long:  `Stops the recording session in the current terminal, or a specific session by ID.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			// Stop specific session by ID
			sessionID := args[0]
			if err := recorder.StopSessionByID(sessionID); err != nil {
				return err
			}
			fmt.Printf("Stopped recording session: %s\n", sessionID)
			return nil
		}

		// Stop session in current terminal
		sessionID, err := recorder.StopCurrentSession()
		if err != nil {
			return err
		}
		fmt.Printf("Stopped recording session: %s\n", sessionID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
