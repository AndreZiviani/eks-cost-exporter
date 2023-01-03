package exporter

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	Namespace           = "eks_cost"
	refreshAfterSeconds = 10
)

func NewMetrics(ctx context.Context, registry *prometheus.Registry) (*Metrics, error) {
	m := Metrics{registry: registry}
	m.Instances = make(map[string]*Instance)
	m.Pods = make(map[string]*Pod)
	m.Nodes = make(map[string]*Node)
	m.Metrics = make(map[string]*prometheus.CounterVec)

	m.init(ctx)

	prometheus.MustRegister(&m)

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

	//m.initializeMetrics()

	m.GetInstances(ctx)
	m.GetFargatePricing(ctx)

	m.GetNodes(ctx)
	m.GetPods(ctx)

	m.GetUsageCost()
}

func (m *Metrics) initializeMetrics() {
	m.Metrics["pod_total"] = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "pod_total",
		Help:      "Total cost of the pod, if requests is bigger than current usage then considers the requests cost.",
	}, podLabels)

	m.Metrics["pod_memory"] = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "pod_memory",
		Help:      "Cost of the pod memory usage.",
	}, podLabels)

	m.Metrics["pod_cpu"] = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "pod_cpu",
		Help:      "Cost of the pod cpu usage.",
	}, podLabels)

	m.Metrics["pod_memory_requests"] = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "pod_memory_requests",
		Help:      "Cost of the pod memory requests.",
	}, podLabels)

	m.Metrics["pod_cpu_requests"] = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "pod_cpu_requests",
		Help:      "Cost of the pod cpu requests.",
	}, podLabels)

	for _, metric := range m.Metrics {
		m.registry.MustRegister(metric)
	}
}

func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, ch)
}

func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	m.GetUsageCost()
	m.podsMtx.Lock()
	for _, pod := range m.Pods {
		ch <- prometheus.MustNewConstMetric(
			podTotalDesc,
			prometheus.CounterValue,
			pod.Cost,
			pod.Name, pod.Namespace, pod.Node.Instance.Kind, pod.Node.Instance.Type,
		)

		ch <- prometheus.MustNewConstMetric(
			podCpuDesc,
			prometheus.CounterValue,
			pod.VCpuCost,
			pod.Name, pod.Namespace, pod.Node.Instance.Kind, pod.Node.Instance.Type,
		)

		ch <- prometheus.MustNewConstMetric(
			podMemoryDesc,
			prometheus.CounterValue,
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
	ch <- prometheus.MustNewConstMetric(
		test,
		prometheus.GaugeValue,
		float64(m.now),
	)
	m.podsMtx.Unlock()
}
