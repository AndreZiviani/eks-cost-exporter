package main

import (
	"context"
	"flag"
	"net/http"
	"strings"

	"github.com/AndreZiviani/eks-cost-exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	addr          = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	metricsPath   = flag.String("metrics-path", "/metrics", "path to metrics endpoint")
	rawLevel      = flag.String("log-level", "info", "log level")
	addPodLabels  = flag.String("add-pod-labels", "", "Comma separated list of pod labels that should be added to the cost_pod metric")
	addNodeLabels = flag.String("add-node-labels", "", "Comma separated list of node labels that should be added to the cost_node metric")
)

func init() {
	flag.Parse()
	parsedLevel, err := log.ParseLevel(*rawLevel)
	if err != nil {
		log.WithError(err).Warnf("Couldn't parse log level, using default: %s", log.GetLevel())
	} else {
		log.SetLevel(parsedLevel)
		log.Debugf("Set log level to %s", parsedLevel)
	}
}

func main() {
	log.Infof("Starting EKS Cost Exporter. [log-level=%s]", *rawLevel)

	ctx := context.TODO()

	registry := prometheus.NewRegistry()

	podLabels := []string{}
	if len(*addPodLabels) > 0 {
		podLabels = strings.Split(strings.ReplaceAll(*addPodLabels, " ", ""), ",")
	}
	nodeLabels := []string{}
	if len(*addNodeLabels) > 0 {
		nodeLabels = strings.Split(strings.ReplaceAll(*addNodeLabels, " ", ""), ",")
	}

	_, err := exporter.NewMetrics(ctx, registry, podLabels, nodeLabels)
	if err != nil {
		log.Fatal(err)
	}

	log.Infof("Starting metric http endpoint [address=%s, path=%s]", *addr, *metricsPath)
	http.Handle(*metricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	http.HandleFunc("/", rootHandler)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<html>
		<head><title>EKS Cost Exporter</title></head>
		<body>
		<h1>EKS Cost Exporter</h1>
		<p><a href="` + *metricsPath + `">Metrics</a></p>
		</body>
		</html>
	`))

}
