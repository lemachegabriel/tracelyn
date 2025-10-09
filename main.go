/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import "tracelyn/cmd"

// Version information set by goreleaser
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
