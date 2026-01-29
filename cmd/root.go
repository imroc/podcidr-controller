package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"

	"github.com/imroc/podcidr-controller/pkg/controller"
	"github.com/imroc/podcidr-controller/pkg/selector"
	"github.com/imroc/podcidr-controller/pkg/taint"
)

var (
	clusterCIDR      string
	nodeCIDRMaskSize int
	nodeSelectorStr  string
	removeTaintsStr  string
	leaderElect      bool
	leaseDuration    time.Duration
	renewDeadline    time.Duration
	retryPeriod      time.Duration
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
	rootCmd.Flags().StringVar(&nodeSelectorStr, "node-selector", "", "JSON array of matchExpressions to filter nodes for CIDR allocation")
	rootCmd.Flags().StringVar(&removeTaintsStr, "remove-taints", "", "Comma-separated taints to remove from nodes (formats: key, key:effect, key=value:effect)")
	rootCmd.Flags().BoolVar(&leaderElect, "leader-elect", true, "Enable leader election for HA")
	rootCmd.Flags().DurationVar(&leaseDuration, "leader-elect-lease-duration", 15*time.Second, "Lease duration for leader election")
	rootCmd.Flags().DurationVar(&renewDeadline, "leader-elect-renew-deadline", 10*time.Second, "Renew deadline for leader election")
	rootCmd.Flags().DurationVar(&retryPeriod, "leader-elect-retry-period", 2*time.Second, "Retry period for leader election")
	_ = rootCmd.MarkFlagRequired("cluster-cidr")
}

func Execute() error {
	return rootCmd.Execute()
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	if leaderElect {
		return runWithLeaderElection(ctx, clientset)
	}
	return runController(ctx, clientset)
}

func runWithLeaderElection(ctx context.Context, clientset kubernetes.Interface) error {
	id, err := os.Hostname()
	if err != nil {
		return err
	}

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "kube-system"
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      "podcidr-controller",
			Namespace: namespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				if err := runController(ctx, clientset); err != nil {
					klog.Fatalf("Controller error: %v", err)
				}
			},
			OnStoppedLeading: func() {
				klog.Info("Lost leadership")
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					return
				}
				klog.Infof("New leader elected: %s", identity)
			},
		},
	})

	return nil
}

func runController(ctx context.Context, clientset kubernetes.Interface) error {
	nodeSelector, err := selector.Parse(nodeSelectorStr)
	if err != nil {
		return fmt.Errorf("failed to parse node-selector: %w", err)
	}

	taintRemover, err := taint.NewTaintRemover(removeTaintsStr)
	if err != nil {
		return fmt.Errorf("failed to parse remove-taints: %w", err)
	}

	informerFactory := informers.NewSharedInformerFactory(clientset, time.Minute*10)

	ctrl, err := controller.NewController(clientset, informerFactory, clusterCIDR, nodeCIDRMaskSize, nodeSelector, taintRemover)
	if err != nil {
		return err
	}

	informerFactory.Start(ctx.Done())

	return ctrl.Run(ctx, 2)
}
