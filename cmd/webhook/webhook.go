package webhook

import (
	server "github.com/h3poteto/aws-global-accelerator-controller/pkg/webhoook"
	"github.com/spf13/cobra"
)

type options struct {
	tlsCertFile string
	tlsKeyFile  string
	enableSSL   bool
	port        int32
}

// +kubebuilder:webhook:path=/validate-endpointgroupbinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=operator.h3poteto.dev,resources=endpointgroupbindings,verbs=create;update,versions=v1alpha1,name=validate-endpointgroupbinding.h3poteto.dev,admissionReviewVersions=v1

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
	flags.Int32Var(&o.port, "port", 8443, "Webhook server port.")
	flags.BoolVar(&o.enableSSL, "ssl", true, "Webhook server use SSL.")

	return cmd
}

func (o *options) run(cmd *cobra.Command, args []string) {
	if o.enableSSL && (o.tlsCertFile == "" || o.tlsKeyFile == "") {
		cmd.PrintErr("You must set --tls-cert-file and --tls-private-key-file when you use SSL\n")
		cmd.Help()
		return
	}

	server.Server(o.port, o.tlsCertFile, o.tlsKeyFile)
}
