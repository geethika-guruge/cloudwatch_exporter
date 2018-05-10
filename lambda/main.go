package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/technofy/cloudwatch_exporter/collector"

	"os"
	"sync"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/technofy/cloudwatch_exporter/config"
)

type Request struct {
	Task    string `json:"task"`
	Region  string `json:"region"`
	RoleArn string `json:"roleArn"`
}

type Response struct {
	Message string `json:"message"`
	Ok      bool   `json:"ok"`
}

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
func handleTarget(request Request) (Response, error) {
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	flag.Parse()

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

	target := "target"
	fmt.Println(request.Task)
	task := request.Task

	region := request.Region
	roleArn := request.RoleArn

	// Check if we have all the required parameters in the URL
	if task == "" {
		//fmt.Fprintln(w, "Error: Missing task parameter")
		fmt.Println("Error: Missing task parameter")
		return Response{
			Message: fmt.Sprintf("Error"),
			Ok:      true,
		}, nil
	}

	configMutex.Lock()
	registry := prometheus.NewRegistry()
	collector, err := collector.NewCwCollector(target, task, region, roleArn, settings)
	if err != nil {
		// Can't create the collector, display error
		//fmt.Fprintf(w, "Error: %s\n", err.Error())
		fmt.Println("Error: %s\n", err.Error())
		configMutex.Unlock()
		return Response{
			Message: fmt.Sprintf("Error"),
			Ok:      true,
		}, nil
	}

	registry.MustRegister(collector)
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		DisableCompression: false,
	})

	// Serve the answer through the Collect method of the Collector
	handler.ServeHTTP(w, req)
	configMutex.Unlock()
	return Response{
		Message: fmt.Sprintf(string(w.Body.Bytes()[:len(w.Body.Bytes())])),
		Ok:      true,
	}, nil
}

func main() {

	lambda.Start(handleTarget)
}
