package exporter

import (
	"context"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	namespace = "eks_cost"
)

func NewMetrics(ctx context.Context, registry *prometheus.Registry, addPodLabels []string, addNodeLabels []string) (*Metrics, error) {
	m := Metrics{}
	m.Instances = make(map[string]*Instance)
	m.Pods = make(map[string]*Pod)
	m.Nodes = make(map[string]*Node)
	m.addPodLabels = addPodLabels
	m.addNodeLabels = addNodeLabels

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

	podLabels := []string{"pod", "namespace", "node", "kind", "type"}
	if len(m.addPodLabels) > 0 {
		for _, v := range m.addPodLabels {
			podLabels = append(podLabels, sanitizeLabel(v))
		}
	}

	for _, pod := range m.Pods {
		podLabelValues := []string{pod.Name, pod.Namespace, pod.Node.Name, pod.Node.Instance.Kind, pod.Node.Instance.Type}
		for _, l := range m.addPodLabels {
			podLabelValues = append(podLabelValues, pod.Labels[l])
		}

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				namespace+"_pod_total",
				"Total cost of the pod, if requests is bigger than current usage then considers the requests cost.",
				podLabels, nil,
			),
			prometheus.GaugeValue,
			pod.Cost,
			podLabelValues...,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				namespace+"_pod_cpu",
				"Cost of the pod cpu usage.",
				podLabels, nil,
			),
			prometheus.GaugeValue,
			pod.VCpuCost,
			podLabelValues...,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				namespace+"_pod_memory",
				"Cost of the pod memory usage.",
				podLabels, nil,
			),
			prometheus.GaugeValue,
			pod.MemoryCost,
			podLabelValues...,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				namespace+"_pod_cpu_requests",
				"Cost of the pod cpu requests.",
				podLabels, nil,
			),
			prometheus.GaugeValue,
			pod.VCpuRequestsCost,
			podLabelValues...,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				namespace+"_pod_memory_requests",
				"Cost of the pod memory requests.",
				podLabels, nil,
			),
			prometheus.GaugeValue,
			pod.MemoryRequestsCost,
			podLabelValues...,
		)
	}
	m.podsMtx.Unlock()

	nodeLabels := []string{"node", "region", "az", "kind", "type"}
	if len(m.addNodeLabels) > 0 {
		for _, v := range m.addNodeLabels {
			nodeLabels = append(nodeLabels, sanitizeLabel(v))
		}
	}

	for _, node := range m.Nodes {
		nodeLabelValues := []string{node.Name, node.Region, node.AZ, node.Instance.Type, node.Instance.Kind}
		for _, l := range m.addNodeLabels {
			nodeLabelValues = append(nodeLabelValues, node.Labels[l])
		}

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				namespace+"_node_total",
				"Total cost of the node",
				nodeLabels, nil,
			),
			prometheus.GaugeValue,
			node.Instance.Cost,
			nodeLabelValues...,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				namespace+"_node_cpu",
				"Cost of node CPU.",
				nodeLabels, nil,
			),
			prometheus.GaugeValue,
			node.Instance.VCpuCost,
			nodeLabelValues...,
		)

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				namespace+"_node_memory",
				"Cost of each node GB of memory",
				nodeLabels, nil,
			),
			prometheus.GaugeValue,
			node.Instance.MemoryCost,
			nodeLabelValues...,
		)
	}
}

func sanitizeLabel(label string) string {
	return strings.Replace(strings.Replace(label, ".", "_", -1), "/", "_", -1)
}
