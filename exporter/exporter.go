package exporter

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	Namespace = "eks_cost"
)

// Exporter implements the prometheus.Exporter interface, and exports AWS Spot Price metrics.
type Exporter struct {
	duration       prometheus.Gauge
	scrapeErrors   prometheus.Gauge
	totalScrapes   prometheus.Counter
	pricingMetrics map[string]*prometheus.GaugeVec
	metrics        *Metrics
	errorCount     uint64
	metricsMtx     sync.RWMutex
	sync.RWMutex
}

type scrapeResult struct {
	Name  string
	Value float64

	Pod       string
	Namespace string
	Kind      string
	Type      string
}

// NewExporter returns a new exporter of AWS EC2 Price metrics.
func NewExporter(ctx context.Context) (*Exporter, error) {

	m, err := NewMetrics(ctx)
	if err != nil {
		return nil, err
	}

	e := Exporter{
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "scrape_duration_seconds",
			Help:      "The scrape duration.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "scrapes_total",
			Help:      "Total AWS autoscaling group scrapes.",
		}),
		scrapeErrors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "scrape_error",
			Help:      "The scrape error status.",
		}),
		metrics: m,
	}

	e.initGauges()
	return &e, nil
}

func (e *Exporter) initGauges() {
	e.pricingMetrics = map[string]*prometheus.GaugeVec{}
	e.pricingMetrics["pod_total"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "pod_total",
		Help:      "Cost of the pod.",
	}, []string{"pod", "namespace", "kind", "type"})

	e.pricingMetrics["pod_memory"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "pod_memory",
		Help:      "Cost of the pod memory usage.",
	}, []string{"pod", "namespace", "kind", "type"})

	e.pricingMetrics["pod_cpu"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "pod_cpu",
		Help:      "Cost of the pod cpu usage.",
	}, []string{"pod", "namespace", "kind", "type"})

	e.pricingMetrics["pod_memory_requests"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "pod_memory_requests",
		Help:      "Cost of the pod memory requests.",
	}, []string{"pod", "namespace", "kind", "type"})

	e.pricingMetrics["pod_cpu_requests"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "pod_cpu_requests",
		Help:      "Cost of the pod cpu requests.",
	}, []string{"pod", "namespace", "kind", "type"})
}

// Describe outputs metric descriptions.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range e.pricingMetrics {
		m.Describe(ch)
	}
	ch <- e.duration.Desc()
	ch <- e.totalScrapes.Desc()
	ch <- e.scrapeErrors.Desc()
}

// Collect fetches info from the AWS API
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	pricingScrapes := make(chan scrapeResult)

	e.Lock()
	defer e.Unlock()

	e.initGauges()
	go e.scrape(pricingScrapes)
	e.setPricingMetrics(pricingScrapes)

	e.duration.Collect(ch)
	e.totalScrapes.Collect(ch)
	e.scrapeErrors.Collect(ch)

	for _, m := range e.pricingMetrics {
		m.Collect(ch)
	}
}
