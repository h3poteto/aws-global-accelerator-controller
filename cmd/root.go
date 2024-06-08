package cmd

import (
	"flag"

	"github.com/h3poteto/aws-global-accelerator-controller/cmd/controller"
	"github.com/h3poteto/aws-global-accelerator-controller/cmd/webhook"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

// RootCmd is cobra command.
var RootCmd = &cobra.Command{
	Use:           "aws-global-accelerator-controller",
	Short:         "aws-global-accelerator-controller is a to manage AWS Global Accelerator from Kubernetes",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	klog.InitFlags(flag.CommandLine)

	cobra.OnInitialize()
	RootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	RootCmd.AddCommand(
		controller.ControllerCmd(),
		webhook.WebhookCmd(),
		versionCmd(),
	)
}
