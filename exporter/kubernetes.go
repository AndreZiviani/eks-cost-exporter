package exporter

import (
	"context"
	"regexp"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
)

var (
	fargateRe = regexp.MustCompile(`(?P<cpu>[0-9.]+?)vCPU (?P<memory>[0-9.]+?)GB`)
)

func (m *Metrics) GetPods(ctx context.Context) {
	now := time.Now()
	defer timeTrack(now, "Retrieving current pod list")

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

	log.Debugf("Pod removed: %s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)

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

	if newPod.Status.Phase == "Pending" {
		return
	}

	log.Debugf("Pod updated: %s/%s", newPod.ObjectMeta.Namespace, newPod.ObjectMeta.Name)

	if len(newPod.Spec.NodeName) > 0 {
		pod := m.Pods[newPod.ObjectMeta.Namespace+"/"+newPod.ObjectMeta.Name]
		if pod == nil {
			// pod changed from Pending to Running
			m.podCreated(newObj)
		} else {
			pod.Node = m.Nodes[newPod.Spec.NodeName]
			m.updatePodCost(pod)
		}
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

	if pod.Status.Phase == "Pending" {
		return
	}

	log.Debugf("Pod created: %s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)

	resources := m.mergeResources(pod.Spec.Containers)
	if m.Nodes[pod.Spec.NodeName] != nil {
		if m.Nodes[pod.Spec.NodeName].Instance.Type == "fargate" {
			// fargate allocates more resources than requested and charges accordingly
			// the allocation size is exposed as an annotation
			// https://docs.aws.amazon.com/eks/latest/userguide/fargate-pod-configuration.html
			annotation := pod.ObjectMeta.Annotations["CapacityProvisioned"]
			r := fargateRe.FindStringSubmatch(annotation)

			cpu, _ := strconv.ParseFloat(r[fargateRe.SubexpIndex("cpu")], 64)
			memory, _ := strconv.ParseFloat(r[fargateRe.SubexpIndex("memory")], 64)

			m.Nodes[pod.Spec.NodeName].Cost.VCpu = m.Instances["fargate"].OnDemandCost.VCpu * cpu
			m.Nodes[pod.Spec.NodeName].Cost.Memory = m.Instances["fargate"].OnDemandCost.Memory * memory
			m.Nodes[pod.Spec.NodeName].Cost.Total = m.Nodes[pod.Spec.NodeName].Cost.VCpu + m.Nodes[pod.Spec.NodeName].Cost.Memory

			cpu = cpu * 1000                     // to millicore
			memory = memory * 1024 * 1024 * 1024 // to GB
			resources.Cpu.SetMilli(int64(cpu))
			resources.Memory.Set(int64(memory))

		}
	}

	tmp := Pod{
		Name:      pod.ObjectMeta.Name,
		Namespace: pod.ObjectMeta.Namespace,
		Labels:    m.exposedPodLabels(pod.ObjectMeta.Labels),
		Resources: resources,
		Node:      m.Nodes[pod.Spec.NodeName],
		Usage: &PodResources{
			Cpu:    resource.NewQuantity(0, resource.DecimalSI),
			Memory: resource.NewQuantity(0, resource.BinarySI),
		},
	}

	m.podsMtx.Lock()
	m.Pods[pod.ObjectMeta.Namespace+"/"+pod.ObjectMeta.Name] = &tmp
	m.updatePodCost(&tmp)
	m.podsMtx.Unlock()
}

func (m *Metrics) GetNodes(ctx context.Context) {
	now := time.Now()
	defer timeTrack(now, "Retrieving current node list")

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

	log.Debugf("Node removed: %s", node.ObjectMeta.Name)

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

	log.Debugf("Node created: %s", node.ObjectMeta.Name)

	tmp := Node{
		Name:   node.ObjectMeta.Name,
		Labels: m.exposedNodeLabels(node.ObjectMeta.Labels),
		AZ:     node.ObjectMeta.Labels["topology.kubernetes.io/zone"],
		Region: node.ObjectMeta.Labels["topology.kubernetes.io/region"],
	}

	if _, ok := node.ObjectMeta.Labels["node.kubernetes.io/instance-type"]; ok {
		// EC2
		tmp.Instance = m.Instances[node.ObjectMeta.Labels["node.kubernetes.io/instance-type"]]

		if _, ok := node.ObjectMeta.Labels["karpenter.sh/capacity-type"]; ok && node.ObjectMeta.Labels["karpenter.sh/capacity-type"] == "spot" {
			// Node managed by Karpenter and is Spot
			tmp.Cost = tmp.Instance.SpotCost[tmp.AZ]
		} else {
			tmp.Cost = tmp.Instance.OnDemandCost
		}
	} else if _, ok := node.Labels["eks.amazonaws.com/compute-type"]; ok && node.Labels["eks.amazonaws.com/compute-type"] == "fargate" {
		// Fargate
		tmp.Instance = m.Instances["fargate"]
		tmp.Cost = &Ec2Cost{Type: "fargate", VCpu: tmp.Instance.OnDemandCost.VCpu, Memory: tmp.Instance.OnDemandCost.Memory}
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

func (m *Metrics) GetUsageCost() {
	podMetricsList, err := m.metrics.MetricsV1beta1().PodMetricses("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	log.Debugf("Refreshing pod usage and cost")

	// caller is already holding the lock
	for _, pod := range podMetricsList.Items {
		name := pod.GetName()
		namespace := pod.GetNamespace()

		me := m.Pods[namespace+"/"+name]
		me.Usage.Cpu.Reset()
		me.Usage.Memory.Reset()

		for _, container := range pod.Containers {
			me.Usage.Cpu.Add(container.Usage["cpu"])
			me.Usage.Memory.Add(container.Usage["memory"])
		}

		m.updatePodCost(me)
	}
}

func (m *Metrics) updatePodCost(pod *Pod) {
	if pod.Node == nil {
		pod.MemoryCost = float64(0)
		pod.VCpuCost = float64(0)
		pod.MemoryRequestsCost = float64(0)
		pod.VCpuRequestsCost = float64(0)

		return
	}

	nodeCost := pod.Node.Cost
	if pod.Node.Cost.Type == "fargate" {
		// since fargate have a fixed price per VCpu/Memory we need to consider that instead of node cost
		// node cost is already scaled to the actual cost instead of base price
		nodeCost = m.Instances["fargate"].OnDemandCost
	}

	// convert bytes to GB
	pod.MemoryCost = float64(pod.Usage.Memory.Value()) / 1024 / 1024 / 1024 * nodeCost.Memory
	pod.MemoryRequestsCost = float64(pod.Resources.Memory.Value()) / 1024 / 1024 / 1024 * nodeCost.Memory

	//convert millicore to core
	pod.VCpuCost = float64(pod.Usage.Cpu.MilliValue()) / 1000 * nodeCost.VCpu
	pod.VCpuRequestsCost = float64(pod.Resources.Cpu.MilliValue()) / 1000 * nodeCost.VCpu

	pod.Cost = max(pod.MemoryCost, pod.MemoryRequestsCost) + max(pod.VCpuCost, pod.VCpuRequestsCost)
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (m Metrics) exposedPodLabels(podLabels map[string]string) map[string]string {
	if len(m.addPodLabels) == 0 {
		return map[string]string{}
	}

	d := make(map[string]string, 0)
	for _, addLabel := range m.addPodLabels {
		if l, ok := podLabels[addLabel]; ok {
			d[addLabel] = l
		}
	}

	return d
}

func (m Metrics) exposedNodeLabels(nodeLabels map[string]string) map[string]string {
	if len(m.addNodeLabels) == 0 {
		return map[string]string{}
	}

	d := make(map[string]string, 0)
	for _, addLabel := range m.addNodeLabels {
		if l, ok := nodeLabels[addLabel]; ok {
			d[addLabel] = l
		}
	}

	return d
}
