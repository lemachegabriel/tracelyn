/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version information set by main
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetVersionInfo sets version information from main package
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tracelyn",
	Short: "Record terminal sessions to clean plain text files",
	Long: `Tracelyn is a CLI tool that records terminal sessions to clean plain text files.

It creates a transparent sub-shell using PTY (pseudo-terminal) that captures all
commands and outputs without interfering with your workflow. All recordings are
saved as clean, readable plain text files.`,
	Version: version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Set custom version template
	rootCmd.SetVersionTemplate(fmt.Sprintf("tracelyn version %s (commit: %s, built: %s)\n", version, commit, date))
}


