package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"

	"../collector"
	"../config"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Request struct {
	Target  string `json:"target"`
	Task    string `json:"task"`
	Region  string `json:"region"`
	RoleArn string `json:"roleArn"`
}

type Response struct {
	ScrapeResult string `json:"scrape_result"`
}

var (
	settings    *config.Settings
	configMutex = &sync.Mutex{}
)

func loadConfigFile() error {
	var err error
	var tmpSettings *config.Settings
	configMutex.Lock()

	// Initial loading of the configuration file
	tmpSettings, err = config.Load("config.yml")
	if err != nil {
		return err
	}

	collector.GenerateTemplates(tmpSettings)

	settings = tmpSettings
	configMutex.Unlock()

	return nil
}

// handleTarget handles scrape requests which make use of CloudWatch service
func handleTarget(request Request) (Response, error) {
	req, reqErr := http.NewRequest("GET", "http://example.com/foo", nil)

	if reqErr != nil {
		fmt.Printf("Can't create new request: %s\n", reqErr.Error())
		os.Exit(-1)
	}

	w := httptest.NewRecorder()

	err := loadConfigFile()
	if err != nil {
		fmt.Printf("Can't read configuration file: %s\n", err.Error())
		os.Exit(-1)
	}

	collector.RegisterGlobalCounter()

	fmt.Println("CloudWatch exporter started...")

	target := request.Target
	task := request.Task

	region := request.Region
	roleArn := request.RoleArn

	// Check if we have all the required parameters in the URL
	if task == "" {
		fmt.Println("Error: Missing task parameter")
		return Response{
			ScrapeResult: "Error",
		}, errors.New("Error: Missing task parameter")
	}

	configMutex.Lock()
	registry := prometheus.NewRegistry()
	collector, err := collector.NewCwCollector(target, task, region, roleArn, settings)
	if err != nil {
		// Can't create the collector, display error
		fmt.Println("Error: %s\n", err.Error())
		configMutex.Unlock()
		return Response{
			ScrapeResult: "Error",
		}, err
	}

	registry.MustRegister(collector)
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		DisableCompression: false,
	})

	// Serve the answer through the Collect method of the Collector
	handler.ServeHTTP(w, req)
	configMutex.Unlock()

	// Print total number of API requests made to CloudWatch.
	fmt.Println(prometheus.DefaultGatherer.Gather())

	return Response{
		ScrapeResult: fmt.Sprintf(string(w.Body.Bytes()[:len(w.Body.Bytes())])),
	}, nil
}

func main() {
	lambda.Start(handleTarget)
}
