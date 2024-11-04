package cmd

import (
	"os"

	"github.com/koki-develop/moview/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:  "moview",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ui.Start(&ui.Option{Path: args[0]}); err != nil {
			return err
		}
		return nil
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
