package cli

import (
	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "trae",
		Short: "Trae Agent — CLI coding agent",
	}
	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewShowConfigCmd())
	return root
}
