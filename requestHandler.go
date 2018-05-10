package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/technofy/cloudwatch_exporter/collector"
	"github.com/technofy/cloudwatch_exporter/config"
)

var (
	listenAddress = flag.String("web.listen-address", ":9042", "Address on which to expose metrics and web interface.")
	metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose exporter's metrics.")
	scrapePath    = flag.String("web.telemetry-scrape-path", "/scrape", "Path under which to expose CloudWatch metrics.")
	configFile    = flag.String("config.file", "config.yml", "Path to configuration file.")

	globalRegistry *prometheus.Registry
	settings       *config.Settings
	totalRequests  prometheus.Counter
	configMutex    = &sync.Mutex{}
)

type Request struct {
	Target  string `json:"target"`
	Task    string `json:"task"`
	Region  string `json:"region"`
	RoleArn string `json:"roleArn"`
}

type Response struct {
	ScrapeResult string `json:"scrape_result"`
	Success      bool   `json:"success"`
}

func loadConfigFile() error {
	var err error
	var tmpSettings *config.Settings
	configMutex.Lock()

	// Initial loading of the configuration file
	tmpSettings, err = config.Load(*configFile)
	if err != nil {
		return err
	}

	collector.GenerateTemplates(tmpSettings)

	settings = tmpSettings
	configMutex.Unlock()

	return nil
}

// handleReload handles a full reload of the configuration file and regenerates the collector templates.
func handleReload(w http.ResponseWriter, req *http.Request) {
	err := loadConfigFile()
	if err != nil {
		str := fmt.Sprintf("Can't read configuration file: %s", err.Error())
		fmt.Fprintln(w, str)
		fmt.Println(str)
	}
	fmt.Fprintln(w, "Reload complete")
}

// handleTarget handles scrape requests which make use of CloudWatch service
func handleTarget(w http.ResponseWriter, req *http.Request) {
	urlQuery := req.URL.Query()

	target := urlQuery.Get("target")
	task := urlQuery.Get("task")
	region := urlQuery.Get("region")
	roleArn := urlQuery.Get("roleArn")

	// Check if we have all the required parameters in the URL
	if task == "" {
		fmt.Fprintln(w, "Error: Missing task parameter")
		return
	}

	configMutex.Lock()
	registry := prometheus.NewRegistry()
	collector, err := collector.NewCwCollector(target, task, region, roleArn, settings)
	if err != nil {
		// Can't create the collector, display error
		fmt.Fprintf(w, "Error: %s\n", err.Error())
		configMutex.Unlock()
		return
	}

	registry.MustRegister(collector)
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		DisableCompression: false,
	})

	// Serve the answer through the Collect method of the Collector
	handler.ServeHTTP(w, req)
	configMutex.Unlock()
}

func lambdaHandler(request Request) (Response, error) {
	req, _ := http.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	flag.Parse()

	err := loadConfigFile()
	if err != nil {
		fmt.Printf("Can't read configuration file: %s\n", err.Error())
		os.Exit(-1)
	}

	fmt.Println("CloudWatch exporter started...")

	target := request.Target
	task := request.Task
	region := request.Region
	roleArn := request.RoleArn

	// Check if we have all the required parameters in the URL
	if task == "" {
		return Response{
			ScrapeResult: fmt.Sprintf("Error"),
			Success:      false,
		}, errors.New("Error: Missing task parameter")
	}
	if roleArn == "" {
		return Response{
			ScrapeResult: fmt.Sprintf("Error"),
			Success:      false,
		}, errors.New("Error: Missing role parameter")
	}

	configMutex.Lock()
	registry := prometheus.NewRegistry()
	collector, err := collector.NewCwCollector(target, task, region, roleArn, settings)
	if err != nil {
		// Can't create the collector, display error
		configMutex.Unlock()
		return Response{
			ScrapeResult: fmt.Sprintf("Error"),
			Success:      false,
		}, err
	}

	registry.MustRegister(collector)
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		DisableCompression: false,
	})

	// Serve the answer through the Collect method of the Collector
	handler.ServeHTTP(w, req)
	configMutex.Unlock()

	s := string(w.Body.Bytes()[:len(w.Body.Bytes())])

	return Response{
		ScrapeResult: s,
		Success:      true,
	}, nil

}
