package cmd

import (
	"fmt"
	"os"

	"github.com/koki-develop/moview/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:  "moview FILE",
	Args: cobra.ExactArgs(1),
	Long: "Play video in terminal.",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := args[0]

		if _, err := os.Stat(p); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file does not exist: %s", p)
			}
			return err
		}

		if err := ui.Start(&ui.Option{Path: p}); err != nil {
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
