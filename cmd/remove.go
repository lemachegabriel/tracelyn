package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"tracelyn/internal/recorder"
)

var removeAllFlag bool

var removeCmd = &cobra.Command{
	Use:   "remove [session-id]",
	Short: "Remove a completed recording session",
	Long:  `Removes a completed session from the registry and deletes its session file.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		registry, err := recorder.LoadSessions()
		if err != nil {
			return err
		}

		// Handle --all flag
		if removeAllFlag {
			removed := 0
			sessions := registry.ListAllSessions()

			for _, s := range sessions {
				if s.Status == "completed" {
					if err := registry.RemoveSession(s.ID); err != nil {
						fmt.Printf("Failed to remove session %s: %v\n", s.ID, err)
						continue
					}
					removed++
					fmt.Printf("Removed session [%s] %s\n", s.ID, s.Name)
				}
			}

			if removed == 0 {
				fmt.Println("No completed sessions to remove")
				return nil
			}

			if err := registry.Save(); err != nil {
				return fmt.Errorf("failed to save registry: %w", err)
			}

			fmt.Printf("\nRemoved %d session(s)\n", removed)
			return nil
		}

		// Require session ID if --all not specified
		if len(args) == 0 {
			return fmt.Errorf("session ID required (or use --all flag)")
		}

		sessionID := args[0]

		// Remove the session
		if err := registry.RemoveSession(sessionID); err != nil {
			return err
		}

		// Save the updated registry
		if err := registry.Save(); err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}

		fmt.Printf("Session [%s] removed successfully\n", sessionID)
		return nil
	},
}

func init() {
	removeCmd.Flags().BoolVarP(&removeAllFlag, "all", "a", false, "Remove all completed sessions")
	rootCmd.AddCommand(removeCmd)
}
