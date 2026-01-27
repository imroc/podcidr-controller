package cmd

import (
	"github.com/spf13/cobra"
)

var (
	clusterCIDR      string
	nodeCIDRMaskSize int
)

var rootCmd = &cobra.Command{
	Use:   "podcidr-controller",
	Short: "A lightweight Pod CIDR allocator for Kubernetes nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return run()
	},
}

func init() {
	rootCmd.Flags().StringVar(&clusterCIDR, "cluster-cidr", "", "CIDR range for pod IPs (required)")
	rootCmd.Flags().IntVar(&nodeCIDRMaskSize, "node-cidr-mask-size", 24, "Mask size for node CIDR")
	rootCmd.MarkFlagRequired("cluster-cidr")
}

func Execute() error {
	return rootCmd.Execute()
}

func run() error {
	// TODO: implement controller logic
	return nil
}
