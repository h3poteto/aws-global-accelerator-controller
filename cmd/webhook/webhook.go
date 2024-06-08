package webhook

import (
	server "github.com/h3poteto/aws-global-accelerator-controller/pkg/webhoook"
	"github.com/spf13/cobra"
)

type options struct {
	tlsCertFile string
	tlsKeyFile  string
}

func WebhookCmd() *cobra.Command {
	o := &options{}
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Start webhook server",
		Run:   o.run,
	}
	flags := cmd.Flags()
	flags.StringVar(&o.tlsCertFile, "tls-cert-file", "", "File containing the x509 Certificate for HTTPS.")
	flags.StringVar(&o.tlsKeyFile, "tls-private-key-file", "", "File containing the x509 private key to --tls-cert-file.")

	return cmd
}

func (o *options) run(cmd *cobra.Command, args []string) {
	if o.tlsCertFile == "" || o.tlsKeyFile == "" {
		cmd.Help()
		return
	}

	server.Server(o.tlsCertFile, o.tlsKeyFile)
}
