package exporter

import (
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
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
	podsMtx     sync.RWMutex
	podsChan    chan struct{}
	podsCached  bool
	nodesMtx    sync.RWMutex
	nodesChan   chan struct{}
	nodesCached bool

	addPodLabels  []string
	addNodeLabels []string
}

type Ec2Cost struct {
	Type   string
	Total  float64
	VCpu   float64
	Memory float64
}

type Instance struct {
	//Kind string
	Type         string
	VCpu         int32
	Memory       int64
	OnDemandCost *Ec2Cost
	SpotCost     map[string]*Ec2Cost
}

type Pod struct {
	Name               string
	Namespace          string
	Labels             map[string]string
	Resources          *PodResources
	Node               *Node
	Usage              *PodResources
	Cost               float64
	VCpuCost           float64
	MemoryCost         float64
	VCpuRequestsCost   float64
	MemoryRequestsCost float64
}

type Node struct {
	Name     string
	Labels   map[string]string
	AZ       string
	Region   string
	Instance *Instance
	Cost     *Ec2Cost
}

type PodResources struct {
	Cpu    *resource.Quantity
	Memory *resource.Quantity
}
