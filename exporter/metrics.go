package exporter

import (
	"context"

	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
)

func NewMetrics(ctx context.Context) (*Metrics, error) {
	m := Metrics{}
	m.Instances = make(map[string]*Instance)
	m.Pods = make(map[string]*Pod)
	m.Nodes = make(map[string]*Node)

	m.init(ctx)

	return &m, nil
}

func (m *Metrics) init(ctx context.Context) {
	config := ctrl.GetConfigOrDie()
	m.config = config

	clientset := kubernetes.NewForConfigOrDie(config)
	m.kubernetes = clientset

	metricsClientset := metricsv.NewForConfigOrDie(config)
	m.metrics = metricsClientset

	cfg, err := newAWSConfig(ctx)
	if err != nil {
		panic(err.Error())
	}
	m.awsconfig = cfg

	m.GetInstances(ctx)
	m.GetFargatePricing(ctx)

}

func (m *Metrics) GetCost(ctx context.Context) {
	m.GetUsage(ctx)

	for _, pod := range m.Pods {
		// convert bytes to GB
		pod.MemoryCost = float64(pod.Usage.Memory.Value()) / 1024 / 1024 / 1024 * pod.Node.Instance.MemoryCost

		//convert millicore to core
		pod.VCpuCost = float64(pod.Usage.Cpu.MilliValue()) / 1000 * pod.Node.Instance.VCpuCost

		pod.Cost = pod.MemoryCost + pod.VCpuCost
	}
}
