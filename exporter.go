package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "twemproxy"
)

type metrics map[string]*prometheus.GaugeVec

var (
	twemproxyLabelNames = []string{"instance"}
	serverLabelNames    = []string{"instance", "group", "redis_server"}
)

func newTwemproxyMetric(metricName string, doc string, constLabels prometheus.Labels) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        "service_" + metricName,
			Help:        doc,
			ConstLabels: constLabels,
		},
		twemproxyLabelNames,
	)
}

func newServerMetric(metricName string, doc string, constLabels prometheus.Labels) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        "server_" + metricName,
			Help:        doc,
			ConstLabels: constLabels,
		},
		serverLabelNames,
	)
}

var (
	twemproxyMetrics = metrics{
		"total_connections":   newTwemproxyMetric("total_connections", "Total connectoins in twemproxy", nil),
		"current_connections": newTwemproxyMetric("current_connections", "Current connections in twemproxy", nil),
	}

	serverMetrics = metrics{
		"in_queue":          newServerMetric("in_queue", "In queue process in redis server", nil),
		"in_queue_bytes":    newServerMetric("in_queue_bytes", "In queue size in redis server", nil),
		"eof":               newServerMetric("eof", "EOF from redis server", nil),
		"err":               newServerMetric("err", "Error from redis server", nil),
		"timed_out":         newServerMetric("timed_out", "Timed out in redis server", nil),
		"server_connection": newServerMetric("connection", "Count of server connection to redis server", nil),
		"server_ejected_at": newServerMetric("ejected_at", "Ejected at time to redis server", nil),
	}
)

var (
	config    = flag.String("config", "", "config path")
	twemphost = flag.String("twemphost", "", "twemproxy host")
	interval  = flag.String("interval", "", "interval of scrap")

	hostname string
)

func registerMetrics(m metrics) error {
	for _, val := range m {
		err := prometheus.Register(val)
		if err != nil {
			return err
		}
	}
	return nil
}

func init() {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		hostname = "unknown_host"
	}

	err = registerMetrics(twemproxyMetrics)
	if err != nil {
		log.Fatal("Canont register Twemproxy metrics ", err.Error())
	}
	err = registerMetrics(serverMetrics)
	if err != nil {
		log.Fatal("Cannot register Redis server metrics ", err.Error())
	}
}

func main() {
	flag.Parse()
	conf, err := LoadConfig(*config)
	if err != nil {
		log.Fatalf("Cannot start twemproxy exporter. Err: %s", err.Error())
	}
	log.Printf("Config: %+v", conf)

	monitor, err := NewMonitor(conf, *twemphost)
	if err != nil {
		log.Fatalf("Cannot create new monitor object. Error: %s", err.Error())
	}

	// exporting metrics by running it using ticker
	stopChan := make(chan bool)
	tickerDuration := time.Second * 3
	if *interval != "" {
		tickerDuration, err = time.ParseDuration(*interval)
		if err != nil {
			log.Fatalf("Cannot parse interval %s. Error: %s", *interval, err.Error())
		}
	}
	ticker := time.NewTicker(tickerDuration)

	go func(ticker *time.Ticker, conf map[string]Config) {
		for {
			select {
			case <-ticker.C:
				err := monitor.Run()
				if err != nil {
					log.Println("Error when running monitor: ", err.Error())
				}
			case <-stopChan:
				return
			}
		}
	}(ticker, conf)

	// expose prometheus endpoint for metrics export
	errChan := make(chan error)
	go func() {
		http.Handle("/metrics", prometheus.Handler())
		err = http.ListenAndServe(":9500", nil)
		if err != nil {
			errChan <- err
		}
	}()

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	select {
	case <-term:
		log.Println("Sigterm detected")
	case err := <-errChan:
		log.Println("Failed to start twemproxy exporter. Error: ", err.Error())
	}

	ticker.Stop()
	stopChan <- true
	log.Println("Twemproxy exporter exited")
}

// Monitor object
type Monitor struct {
	Config  map[string]Config
	tcpHost string
}

// NewMonitor object
func NewMonitor(conf map[string]Config, host string) (Monitor, error) {
	m := Monitor{}
	// set host to localhost:2222 if host is not exists (default port of nutcracker)
	if host == "" {
		host = "localhost:22222"
	}
	m.Config = conf
	m.tcpHost = host
	return m, nil
}

// Run monitoring
func (m *Monitor) Run() error {
	conn, err := net.Dial("tcp", m.tcpHost)
	if err != nil {
		log.Printf("Error when dialing tcp %s. Error: %s", m.tcpHost, err.Error())
	}
	reply := make([]byte, 8192) // at least 8KB

	length, err := conn.Read(reply)
	if err != nil {
		log.Println("Error when read reply from tcp ", err.Error())
	}

	stats, err := parseStats(reply[:length], m.Config)
	if err != nil {
		log.Println("Failed to parse stats: ", err.Error())
	}

	twemproxyMetrics["total_connections"].WithLabelValues(hostname).Set(stats.TotalConnections)
	twemproxyMetrics["current_connections"].WithLabelValues(hostname).Set(stats.CurrentConnections)
	for serviceName, service := range stats.Services {
		for _, server := range service.Servers {
			serverMetrics["in_queue"].WithLabelValues(hostname, serviceName, server.HostAlias).Set(server.InQueue)
			serverMetrics["in_queue_bytes"].WithLabelValues(hostname, serviceName, server.HostAlias).Set(server.InQueueBytes)
			serverMetrics["err"].WithLabelValues(hostname, serviceName, server.HostAlias).Set(server.ServerErr)
			serverMetrics["eof"].WithLabelValues(hostname, serviceName, server.HostAlias).Set(server.ServerEOF)
			serverMetrics["timed_out"].WithLabelValues(hostname, serviceName, server.HostAlias).Set(server.ServerTimedout)
			serverMetrics["server_connection"].WithLabelValues(hostname, serviceName, server.HostAlias).Set(server.ServerConnections)
			serverMetrics["server_ejected_at"].WithLabelValues(hostname, serviceName, server.HostAlias).Set(server.ServerEjectedAt)
		}
	}
	return nil
}
