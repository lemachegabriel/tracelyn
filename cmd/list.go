package cmd

import (
	"fmt"

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

		fmt.Println("Recording sessions:")
		for _, s := range sessions {
			if s.Status == "active" {
				fmt.Printf("  [%s] ✓ ACTIVE   %s (PID: %d)\n", s.ID, s.Name, s.PID)
			} else {
				fmt.Printf("  [%s] ● COMPLETED %s\n", s.ID, s.Name)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
