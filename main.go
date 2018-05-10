package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {

	globalRegistry = prometheus.NewRegistry()

	totalRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudwatch_requests_total",
		Help: "API requests made to CloudWatch",
	})

	globalRegistry.MustRegister(totalRequests)

	prometheus.DefaultGatherer = globalRegistry

	err := loadConfigFile()
	if err != nil {
		fmt.Printf("Can't read configuration file: %s\n", err.Error())
		os.Exit(-1)
	}

	fmt.Println("CloudWatch exporter started...")

	// Expose the exporter's own metrics on /metrics
	http.Handle(*metricsPath, promhttp.Handler())

	// Expose CloudWatch through this endpoint
	http.HandleFunc(*scrapePath, handleTarget)

	// Allows manual reload of the configuration
	http.HandleFunc("/reload", handleReload)

	// Start serving for clients
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
