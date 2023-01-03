package exporter

import (
	"bytes"
	"context"
	"fmt"
	"time"

	promgo "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
)

func (m *Metrics) GetPods(ctx context.Context) {
	m.podsCached = false
	watchlist := cache.NewListWatchFromClient(
		m.kubernetes.CoreV1().RESTClient(),
		"pods", metav1.NamespaceAll,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&corev1.Pod{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    m.podCreated,
			DeleteFunc: m.podRemoved,
			UpdateFunc: m.podUpdated,
		},
	)

	m.podsChan = make(chan struct{})
	go controller.Run(m.podsChan)

	cached := cache.WaitForCacheSync(m.podsChan, controller.HasSynced)
	m.podsMtx.Lock()
	m.podsCached = cached
	m.podsMtx.Unlock()
}

func (m *Metrics) podRemoved(obj interface{}) {
	m.podsMtx.RLock()
	c := m.podsCached
	m.podsMtx.RUnlock()
	if !c {
		return
	}

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	if _, ok := m.Pods[pod.ObjectMeta.Namespace+"/"+pod.ObjectMeta.Name]; ok {
		m.podsMtx.Lock()
		delete(m.Pods, pod.ObjectMeta.Namespace+"/"+pod.ObjectMeta.Name)
		m.podsMtx.Unlock()
	}
}

func (m *Metrics) podUpdated(oldObj, newObj interface{}) {
	m.podsMtx.RLock()
	c := m.podsCached
	m.podsMtx.RUnlock()
	if !c {
		return
	}

	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}

	if len(newPod.Spec.NodeName) > 0 {
		pod := m.Pods[newPod.ObjectMeta.Namespace+"/"+newPod.ObjectMeta.Name]
		pod.Node = m.Nodes[newPod.Spec.NodeName]
		m.updatePodCost(pod)
		return
	}
}

func (m *Metrics) podCreated(obj interface{}) {
	// we actually want to be called when initially populating the cache
	// in order to populate or internal structures

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	tmp := Pod{
		Name:      pod.ObjectMeta.Name,
		Namespace: pod.ObjectMeta.Namespace,
		Resources: m.mergeResources(pod.Spec.Containers),
		Node:      m.Nodes[pod.Spec.NodeName],
		Usage: &PodUsage{
			Cpu:            resource.NewQuantity(0, resource.DecimalSI),
			CpuPrevious:    resource.NewQuantity(0, resource.DecimalSI),
			Memory:         resource.NewQuantity(0, resource.BinarySI),
			MemoryPrevious: resource.NewQuantity(0, resource.BinarySI),
		},
	}

	m.podsMtx.Lock()
	m.Pods[pod.ObjectMeta.Namespace+"/"+pod.ObjectMeta.Name] = &tmp
	m.updatePodCost(&tmp)
	m.podsMtx.Unlock()
}

func (m *Metrics) GetNodes(ctx context.Context) {
	m.nodesCached = false
	watchlist := cache.NewListWatchFromClient(
		m.kubernetes.CoreV1().RESTClient(),
		"nodes", metav1.NamespaceAll,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&corev1.Node{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    m.nodeCreated,
			DeleteFunc: m.nodeRemoved,
		},
	)

	m.nodesChan = make(chan struct{})
	go controller.Run(m.nodesChan)

	cached := cache.WaitForCacheSync(m.nodesChan, controller.HasSynced)
	m.nodesMtx.Lock()
	m.nodesCached = cached
	m.nodesMtx.Unlock()
}

func (m *Metrics) nodeRemoved(obj interface{}) {
	m.nodesMtx.RLock()
	defer m.nodesMtx.RUnlock()
	if !m.nodesCached {
		return
	}

	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}

	if _, ok := m.Nodes[node.ObjectMeta.Name]; ok {
		m.nodesMtx.Lock()
		delete(m.Nodes, node.ObjectMeta.Name)
		m.nodesMtx.Unlock()
	}
}

func (m *Metrics) nodeCreated(obj interface{}) {
	// we actually want to be called when initially populating the cache
	// in order to populate or internal structures
	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}

	var tmp Node
	if _, ok := node.Labels["eks.amazonaws.com/compute-type"]; ok {
		if node.Labels["eks.amazonaws.com/compute-type"] == "fargate" {
			tmp = Node{
				Name:     node.ObjectMeta.Name,
				AZ:       node.ObjectMeta.Labels["topology.kubernetes.io/zone"],
				Region:   node.ObjectMeta.Labels["topology.kubernetes.io/region"],
				Instance: m.Instances["fargate"],
			}
		}
	} else if _, ok := node.ObjectMeta.Labels["node.kubernetes.io/instance-type"]; ok {
		tmp = Node{
			Name:     node.ObjectMeta.Name,
			AZ:       node.ObjectMeta.Labels["topology.kubernetes.io/zone"],
			Region:   node.ObjectMeta.Labels["topology.kubernetes.io/region"],
			Instance: m.Instances[node.ObjectMeta.Labels["node.kubernetes.io/instance-type"]],
		}
	}

	m.nodesMtx.Lock()
	m.Nodes[node.ObjectMeta.Name] = &tmp
	m.nodesMtx.Unlock()
}

func (m Metrics) mergeResources(containers []corev1.Container) *PodResources {
	//TODO: dont allocate if pod does not have resources configured
	resources := PodResources{
		Cpu:    resource.NewQuantity(0, resource.DecimalSI),
		Memory: resource.NewQuantity(0, resource.BinarySI),
	}

	for _, container := range containers {
		if container.Resources.Requests != nil {
			if cpu, ok := container.Resources.Requests["cpu"]; ok {
				resources.Cpu.Add(cpu)
			}
			if memory, ok := container.Resources.Requests["memory"]; ok {
				resources.Memory.Add(memory)
			}
		}
	}

	return &resources
}

func (m *Metrics) updatePodCost(pod *Pod) {
	if pod.Node == nil {
		pod.MemoryCost = float64(0)
		pod.VCpuCost = float64(0)
		pod.MemoryRequestsCost = float64(0)
		pod.VCpuRequestsCost = float64(0)

		return
	}

	var total float64

	if pod.Node.Instance.Type == "fargate" {
		// TODO: get annotation with allocated resources
		return
	}

	if pod.lastScrape.IsZero() {
		pod.lastScrape = time.Now()
		return
	}

	diff := float64(pod.now.Sub(pod.lastScrape).Milliseconds())
	pod.lastScrape = pod.now

	// convert bytes to GB
	gb := float64(1) / 1024 / 1024 / 1024

	// get how many GBs of RAM was used since last scrape in milliseconds
	usage := diff * (float64(pod.Usage.Memory.Value()) * gb)
	usageCost := usage * pod.Node.Instance.MemoryCost / 1000 // to seconds
	pod.MemoryCost += usageCost
	total += usageCost

	requests := diff * (float64(pod.Resources.Memory.Value()) * gb)
	requestsCost := requests * pod.Node.Instance.MemoryCost / 1000 // to seconds
	pod.MemoryRequestsCost += requestsCost
	total += requestsCost

	// pod used more resources than requested
	if pod.Usage.Memory.Cmp(*pod.Resources.Memory) == 1 {
		pod.Cost += usageCost
	} else {
		pod.Cost += requestsCost
	}

	usage = float64((pod.Usage.Cpu.Value() - pod.Usage.CpuPrevious.Value()) / int64(pod.Node.Instance.VCpu))
	usageCost = usage * pod.Node.Instance.VCpuCost / 1000 // to seconds
	pod.VCpuCost += usageCost
	total += usageCost

	requests = diff * float64(pod.Resources.Cpu.Value())
	requestsCost = requests * pod.Node.Instance.VCpuCost / 1000 // to seconds
	pod.VCpuRequestsCost += requestsCost
	total += requestsCost

	// pod used more resources than requested
	if usage > float64(pod.Resources.Cpu.Value()) {
		pod.Cost += usageCost
		total += usageCost
	} else {
		pod.Cost += requestsCost
		total += requestsCost
	}

}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (m *Metrics) GetUsageCost() {
	m.podsMtx.Lock()
	m.nodesMtx.Lock()

	now := time.Now()

	for _, node := range m.Nodes {
		podMetrics := m.kubernetes.CoreV1().RESTClient().Get().Resource("nodes").Name(node.Name).SubResource("proxy").Suffix("metrics/resource")
		r, err := podMetrics.DoRaw(context.TODO())
		if err != nil {
			panic(err)
		}

		reader := bytes.NewReader(r)

		var parser expfmt.TextParser
		mf, err := parser.TextToMetricFamilies(reader)

		for _, metric := range mf["pod_cpu_usage_seconds_total"].Metric {
			pod, err := getPodName(metric)
			if err != nil {
				continue
			}

			me := m.Pods[pod]
			if me.Usage.Cpu.IsZero() {
				me.Usage.CpuPrevious.Set(int64(*metric.Counter.Value))
			} else {
				me.Usage.CpuPrevious.Set(me.Usage.Cpu.Value())
			}
			me.Usage.Cpu.Set(int64(*metric.Counter.Value))
			me.now = now
		}
		for _, metric := range mf["pod_memory_working_set_bytes"].Metric {
			pod, err := getPodName(metric)
			if err != nil {
				continue
			}

			me := m.Pods[pod]
			if me.Usage.Memory.IsZero() {
				me.Usage.Memory.Set(int64(*metric.Gauge.Value))
			} else {
				me.Usage.MemoryPrevious.Set(me.Usage.Memory.Value())
			}
			me.Usage.Memory.Set(int64(*metric.Gauge.Value))
			me.now = now
		}
	}
	m.nodesMtx.Unlock()
	for _, pod := range m.Pods {
		m.updatePodCost(pod)
	}
	m.podsMtx.Unlock()
}

func getPodName(m *promgo.Metric) (string, error) {
	var name, namespace string
	for _, lp := range m.Label {
		if lp.GetName() == "pod" {
			name = lp.GetValue()
		}
		if lp.GetName() == "namespace" {
			namespace = lp.GetValue()
		}
	}

	if len(name) > 0 && len(namespace) > 0 {
		return namespace + "/" + name, nil
	}

	return "", fmt.Errorf("missing required labels")
}
