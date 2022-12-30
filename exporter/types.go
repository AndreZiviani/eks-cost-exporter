package exporter

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Metrics struct {
	Instances map[string]*Instance
	Pods      map[string]*Pod
	Nodes     map[string]*Node

	awsconfig  aws.Config
	config     *rest.Config
	kubernetes *kubernetes.Clientset
	metrics    *metricsv.Clientset
}

type Instance struct {
	VCpu       int32
	Memory     int64
	Cost       float64
	VCpuCost   float64
	MemoryCost float64
}

type Pod struct {
	Name       string
	Namespace  string
	Resources  *PodResources
	Node       *Node
	Usage      *PodResources
	Cost       float64
	VCpuCost   float64
	MemoryCost float64
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
