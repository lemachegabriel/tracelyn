package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"tracelyn/internal/recorder"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all recording sessions",
	Long:  `Shows all recording sessions (active and completed) with their IDs and status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		registry, err := recorder.LoadSessions()
		if err != nil {
			return err
		}

		sessions := registry.ListAllSessions()

		if len(sessions) == 0 {
			fmt.Println("No recording sessions found")
			return nil
		}

		// Get current terminal's session ID if any
		currentSessionID := os.Getenv("TRACELYN_SESSION_ID")

		fmt.Println("Recording sessions:")
		for _, s := range sessions {
			// Check if this is the current terminal's session
			currentTerminal := ""
			if s.ID == currentSessionID {
				currentTerminal = " ← current terminal"
			}

			if s.Status == "active" {
				fmt.Printf("  [%s] ✓ ACTIVE   %s (PID: %d)%s\n", s.ID, s.Name, s.PID, currentTerminal)
			} else {
				fmt.Printf("  [%s] ● COMPLETED %s%s\n", s.ID, s.Name, currentTerminal)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
