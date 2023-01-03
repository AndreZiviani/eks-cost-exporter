package exporter

import (
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

var (
	namespace    = "eks_cost"
	podLabels    = []string{"pod", "namespace", "kind", "type"}
	podTotalDesc = prometheus.NewDesc(
		namespace+"_pod_total",
		"Total cost of the pod, if requests is bigger than current usage then considers the requests cost.",
		podLabels, nil,
	)
	podMemoryDesc = prometheus.NewDesc(
		namespace+"_pod_memory",
		"Cost of the pod memory usage.",
		podLabels, nil,
	)
	podCpuDesc = prometheus.NewDesc(
		namespace+"_pod_cpu",
		"Cost of the pod cpu usage.",
		podLabels, nil,
	)
	podMemoryRequestsDesc = prometheus.NewDesc(
		namespace+"_pod_memory_requests",
		"Cost of the pod memory requests.",
		podLabels, nil,
	)
	podCpuRequestsDesc = prometheus.NewDesc(
		namespace+"_pod_cpu_requests",
		"Cost of the pod cpu requests.",
		podLabels, nil,
	)
	test = prometheus.NewDesc(
		namespace+"_test123",
		"test",
		nil, nil,
	)
)

type Metrics struct {
	Instances map[string]*Instance
	Pods      map[string]*Pod
	Nodes     map[string]*Node
	Metrics   map[string]*prometheus.CounterVec

	awsconfig   aws.Config
	config      *rest.Config
	kubernetes  *kubernetes.Clientset
	metrics     *metricsv.Clientset
	registry    *prometheus.Registry
	podsMtx     sync.RWMutex
	podsChan    chan struct{}
	podsCached  bool
	nodesMtx    sync.RWMutex
	nodesChan   chan struct{}
	nodesCached bool

	now  int64
	last time.Time
}

type Instance struct {
	Kind       string
	Type       string
	VCpu       int32
	Memory     int64
	Cost       float64
	VCpuCost   float64
	MemoryCost float64
}

type Pod struct {
	Name               string
	Namespace          string
	Resources          *PodResources
	Node               *Node
	Usage              *PodUsage
	Cost               float64
	VCpuCost           float64
	MemoryCost         float64
	VCpuRequestsCost   float64
	MemoryRequestsCost float64
	lastScrape         time.Time
	now                time.Time
}

type Node struct {
	Name     string
	AZ       string
	Region   string
	Instance *Instance
}

type PodResources struct {
	Cpu    *resource.Quantity
	Memory *resource.Quantity
}

type PodUsage struct {
	Cpu            *resource.Quantity
	CpuPrevious    *resource.Quantity
	Memory         *resource.Quantity
	MemoryPrevious *resource.Quantity
}
