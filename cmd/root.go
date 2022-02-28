package cmd

import (
	"github.com/h3poteto/aws-global-accelerator-controller/cmd/controller"
	"github.com/spf13/cobra"
)

// RootCmd is cobra command.
var RootCmd = &cobra.Command{
	Use:           "aws-global-accelerator-controller",
	Short:         "aws-global-accelerator-controller is a to manage AWS Global Accelerator from Kubernetes",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	cobra.OnInitialize()
	RootCmd.AddCommand(
		controller.ControllerCmd(),
	)
}
