package main

import (
	"encoding/json"
	"log"
)

// TwemproxyStats to export to prometheus
type TwemproxyStats struct {
	Service            string
	Source             string
	TotalConnections   float64
	CurrentConnections float64
	ExpectedAvailable  int
	NotAvailable       int
	Services           map[string]ServiceStats
}

// ServiceStats for twemproxy
type ServiceStats struct {
	Name              string
	ClientEOF         float64
	ClientErr         float64
	ClientConnections float64
	ServerEjects      float64
	ForwardError      float64
	Fragments         float64
	ExpectedAvailable int
	NotAvailable      int
	Servers           map[string]ServerStats
}

// ServerStats for connection stats
type ServerStats struct {
	Host              string
	HostAlias         string
	ServerEOF         float64 `json:"server_eof,omitempty"`
	ServerErr         float64 `json:"server_err,omitempty"`
	ServerTimedout    float64 `json:"server_timeout,omitempty"`
	ServerConnections float64 `json:"server_connections,omitempty"`
	ServerEjectedAt   float64 `json:"server_ejected_at,omitempty"`
	Requests          float64 `json:"requests,omitempty"`
	RequestBytes      float64 `json:"request_bytes,omitempty"`
	Responses         float64 `json:"responses,omitempty"`
	ResponseBytes     float64 `json:"response_bytes,omitempty"`
	InQueue           float64 `json:"in_queue,omitempty"`
	InQueueBytes      float64 `json:"in_queue_bytes,omitempty"`
	OutQueue          float64 `json:"out_queue,omitempty"`
	OutQueueBytes     float64 `json:"out_queue_bytes,omitempty"`
}

func parseStats(statsContent []byte, config map[string]Config) (TwemproxyStats, error) {
	stats := make(map[string]interface{})
	err := json.Unmarshal(statsContent, &stats)
	if err != nil {
		log.Printf("Content: %v", string(statsContent))
		log.Println("Failed to unmarshal JSON ", err.Error())
		return TwemproxyStats{}, err
	}

	// set the main stats for twemproxy
	twemp := TwemproxyStats{
		Service:            stats["service"].(string),
		Source:             stats["source"].(string),
		TotalConnections:   stats["total_connections"].(float64),
		CurrentConnections: stats["curr_connections"].(float64),
		Services:           make(map[string]ServiceStats),
	}

	for key := range config {
		serviceStats := ServiceStats{
			Name:              key,
			ExpectedAvailable: len(config[key].Servers),
			Servers:           make(map[string]ServerStats),
		}
		s, ok := stats[key]
		if !ok {
			continue
		}
		twemp.ExpectedAvailable += len(config[key].Servers)
		// cast to map[string]interface{}
		service := s.(map[string]interface{})

		// extract vars for service stats
		serviceStats.ClientEOF = service["client_eof"].(float64)
		serviceStats.ClientErr = service["client_err"].(float64)
		serviceStats.ClientConnections = service["client_connections"].(float64)
		serviceStats.ServerEjects = service["server_ejects"].(float64)
		serviceStats.ForwardError = service["forward_error"].(float64)
		serviceStats.Fragments = service["fragments"].(float64)

		for _, val := range config[key].Servers {
			host := val.IP
			hostAlias := val.IP
			if val.Alias != "" {
				host = val.Alias
				//hostAlias += fmt.Sprintf(" (%s)", val.Alias)
			}
			se, ok := service[host]
			if !ok {
				twemp.NotAvailable++
				serviceStats.NotAvailable++
				continue
			}
			srv := se.(map[string]interface{})
			serverStats := ServerStats{
				Host:              host,
				HostAlias:         hostAlias,
				ServerEOF:         srv["server_eof"].(float64),
				ServerErr:         srv["server_err"].(float64),
				ServerTimedout:    srv["server_timedout"].(float64),
				ServerConnections: srv["server_connections"].(float64),
				ServerEjectedAt:   srv["server_ejected_at"].(float64),
				Requests:          srv["requests"].(float64),
				RequestBytes:      srv["request_bytes"].(float64),
				Responses:         srv["responses"].(float64),
				ResponseBytes:     srv["response_bytes"].(float64),
				InQueue:           srv["in_queue"].(float64),
				InQueueBytes:      srv["in_queue_bytes"].(float64),
				OutQueue:          srv["out_queue"].(float64),
				OutQueueBytes:     srv["out_queue_bytes"].(float64),
			}
			serviceStats.Servers[host] = serverStats

			// means there is no connection to the server
			if serverStats.ServerConnections < 1 {
				twemp.NotAvailable++
				serviceStats.NotAvailable++
			}
		}
		twemp.Services[key] = serviceStats
	}
	return twemp, nil
}
