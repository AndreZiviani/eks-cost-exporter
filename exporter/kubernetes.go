package exporter

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *Metrics) GetPods(ctx context.Context) {
	pods, err := m.kubernetes.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, pod := range pods.Items {
		//TODO: check if anything changed (timestamp?) instead of always overwriting
		//TODO: fargate
		m.Pods[pod.ObjectMeta.Name] = &Pod{
			Name:      pod.ObjectMeta.Name,
			Namespace: pod.ObjectMeta.Namespace,
			Resources: m.mergeResources(pod.Spec.Containers),
			Node:      m.Nodes[pod.Spec.NodeName],
			Usage: &PodResources{
				Cpu:    resource.NewQuantity(0, resource.DecimalSI),
				Memory: resource.NewQuantity(0, resource.BinarySI),
			},
		}
	}
}

func (m *Metrics) GetNodes(ctx context.Context) {
	nodes, err := m.kubernetes.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, node := range nodes.Items {
		//TODO: check if anything changed (timestamp?) instead of always overwriting
		//TODO: fargate
		m.Nodes[node.ObjectMeta.Name] = &Node{
			Name:     node.ObjectMeta.Name,
			AZ:       node.ObjectMeta.Labels["topology.kubernetes.io/zone"],
			Region:   node.ObjectMeta.Labels["topology.kubernetes.io/region"],
			Instance: m.Instances[node.ObjectMeta.Labels["node.kubernetes.io/instance-type"]],
		}
	}
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

func (m *Metrics) GetUsage(ctx context.Context) {
	m.GetNodes(ctx)
	m.GetPods(ctx)
	podMetricsList, err := m.metrics.MetricsV1beta1().PodMetricses("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, pod := range podMetricsList.Items {
		name := pod.GetName()

		m.Pods[name].Usage.Cpu.Reset()
		m.Pods[name].Usage.Memory.Reset()

		for _, container := range pod.Containers {
			m.Pods[name].Usage.Cpu.Add(container.Usage["cpu"])
			m.Pods[name].Usage.Memory.Add(container.Usage["memory"])
		}
	}
}
