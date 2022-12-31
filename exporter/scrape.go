package exporter

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func (e *Exporter) scrape(scrapes chan<- scrapeResult) {

	defer close(scrapes)
	now := time.Now()

	e.totalScrapes.Inc()

	var errorCount uint64

	e.metrics.GetUsageCost(context.TODO())

	for _, pod := range e.metrics.Pods {
		scrapes <- scrapeResult{
			Name:      "pod_total",
			Value:     pod.Cost,
			Pod:       pod.Name,
			Namespace: pod.Namespace,
			Kind:      pod.Node.Instance.Kind,
			Type:      pod.Node.Instance.Type,
		}
		scrapes <- scrapeResult{
			Name:      "pod_cpu",
			Value:     pod.VCpuCost,
			Pod:       pod.Name,
			Namespace: pod.Namespace,
			Kind:      pod.Node.Instance.Kind,
			Type:      pod.Node.Instance.Type,
		}
		scrapes <- scrapeResult{
			Name:      "pod_memory",
			Value:     pod.MemoryCost,
			Pod:       pod.Name,
			Namespace: pod.Namespace,
			Kind:      pod.Node.Instance.Kind,
			Type:      pod.Node.Instance.Type,
		}
		scrapes <- scrapeResult{
			Name:      "pod_cpu_requests",
			Value:     pod.VCpuRequestsCost,
			Pod:       pod.Name,
			Namespace: pod.Namespace,
			Kind:      pod.Node.Instance.Kind,
			Type:      pod.Node.Instance.Type,
		}
		scrapes <- scrapeResult{
			Name:      "pod_memory_requests",
			Value:     pod.MemoryRequestsCost,
			Pod:       pod.Name,
			Namespace: pod.Namespace,
			Kind:      pod.Node.Instance.Kind,
			Type:      pod.Node.Instance.Type,
		}
	}

	e.scrapeErrors.Set(float64(atomic.LoadUint64(&errorCount)))
	e.duration.Set(float64(time.Now().UnixNano()-now.UnixNano()) / 1_000_000_000)
}

func (e *Exporter) setPricingMetrics(scrapes <-chan scrapeResult) {
	for scr := range scrapes {
		name := scr.Name
		if _, ok := e.pricingMetrics[name]; !ok {
			e.metricsMtx.Lock()
			//defer e.metricsMtx.Unlock()
			e.pricingMetrics[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      name,
			}, []string{"pod", "namespace", "kind", "type"})
			e.metricsMtx.Unlock()
		}
		var labels prometheus.Labels
		labels = map[string]string{
			"pod":       scr.Pod,
			"namespace": scr.Namespace,
			"kind":      scr.Kind,
			"type":      scr.Type,
		}
		e.pricingMetrics[name].With(labels).Set(float64(scr.Value))
	}
}
