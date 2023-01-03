package exporter

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	Namespace = "eks_cost"
)

func NewMetrics(ctx context.Context, registry *prometheus.Registry) (*Metrics, error) {
	m := Metrics{}
	m.Instances = make(map[string]*Instance)
	m.Pods = make(map[string]*Pod)
	m.Nodes = make(map[string]*Node)

	m.init(ctx)

	registry.MustRegister(&m)
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())

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

	m.GetNodes(ctx)
	m.GetPods(ctx)
}

func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, ch)
}

func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	m.podsMtx.Lock()
	m.GetUsageCost()
	for _, pod := range m.Pods {
		ch <- prometheus.MustNewConstMetric(
			podTotalDesc,
			prometheus.GaugeValue,
			pod.Cost,
			pod.Name, pod.Namespace, pod.Node.Instance.Kind, pod.Node.Instance.Type,
		)

		ch <- prometheus.MustNewConstMetric(
			podCpuDesc,
			prometheus.GaugeValue,
			pod.VCpuCost,
			pod.Name, pod.Namespace, pod.Node.Instance.Kind, pod.Node.Instance.Type,
		)

		ch <- prometheus.MustNewConstMetric(
			podMemoryDesc,
			prometheus.GaugeValue,
			pod.MemoryCost,
			pod.Name, pod.Namespace, pod.Node.Instance.Kind, pod.Node.Instance.Type,
		)

		ch <- prometheus.MustNewConstMetric(
			podCpuRequestsDesc,
			prometheus.GaugeValue,
			pod.VCpuRequestsCost,
			pod.Name, pod.Namespace, pod.Node.Instance.Kind, pod.Node.Instance.Type,
		)

		ch <- prometheus.MustNewConstMetric(
			podMemoryRequestsDesc,
			prometheus.GaugeValue,
			pod.MemoryRequestsCost,
			pod.Name, pod.Namespace, pod.Node.Instance.Kind, pod.Node.Instance.Type,
		)
	}
	m.podsMtx.Unlock()
	for _, node := range m.Nodes {
		ch <- prometheus.MustNewConstMetric(
			nodeTotalDesc,
			prometheus.GaugeValue,
			node.Instance.Cost,
			node.Name, node.Region, node.AZ, node.Instance.Type, node.Instance.Kind,
		)

		ch <- prometheus.MustNewConstMetric(
			nodeVCpuDesc,
			prometheus.GaugeValue,
			node.Instance.VCpuCost,
			node.Name, node.Region, node.AZ, node.Instance.Type, node.Instance.Kind,
		)

		ch <- prometheus.MustNewConstMetric(
			nodeMemoryDesc,
			prometheus.GaugeValue,
			node.Instance.MemoryCost,
			node.Name, node.Region, node.AZ, node.Instance.Type, node.Instance.Kind,
		)
	}
}
