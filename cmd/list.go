package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tracelyn/internal/recorder"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active recording sessions",
	Long:  `Shows all currently active recording sessions with their IDs and PIDs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		registry, err := recorder.LoadSessions()
		if err != nil {
			return err
		}

		sessions := registry.ListActiveSessions()

		if len(sessions) == 0 {
			fmt.Println("No active recording sessions")
			return nil
		}

		fmt.Println("Active recording sessions:")
		for _, s := range sessions {
			fmt.Printf("  [%s] %s (PID: %d)\n", s.ID, s.Name, s.PID)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
