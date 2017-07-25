package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"gopkg.in/yaml.v2"
)

// Error list
var (
	ErrPathEmpty         = errors.New("Config path is empty")
	ErrNoServersDetected = errors.New("No servers detected in config")
)

// Config of twemproxy
type Config struct {
	ConfigName     string // configuration name
	Hash           string
	HashTag        string
	Distribution   string
	AutoEjectHosts bool
	Timeout        int
	Protocol       string
	Redis          bool
	Servers        []Server // one service of twemproxy can have many different redis servers
}

// Server for redis server list
type Server struct {
	IP    string
	Alias string
}

// LoadConfig for twemproxy yaml
func LoadConfig(path string) (map[string]Config, error) {
	if path == "" {
		return nil, ErrPathEmpty
	}

	confContent, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Cannot open: %s. Error: %s", path, err.Error())
	}

	confMap := make(map[string]interface{})
	err = yaml.Unmarshal(confContent, &confMap)
	if err != nil {
		return nil, err
	}

	confs := make(map[string]Config)
	// config name will always be 1
	for key := range confMap {
		confs[key] = Config{ConfigName: key}
	}

	serversExists := false
	// extract variables
	for key := range confs {
		vars := confMap[key].(map[interface{}]interface{})
		// copy conf to var
		c := confs[key]
		c.Hash = vars["hash"].(string)
		c.HashTag = vars["hash_tag"].(string)
		c.Distribution = vars["distribution"].(string)
		c.AutoEjectHosts = vars["auto_eject_hosts"].(bool)
		c.Timeout = vars["timeout"].(int)

		// for protocol and redis schema
		if val, ok := vars["redis"]; ok {
			c.Redis = val.(bool)
		}
		if val, ok := vars["protocol"]; ok {
			c.Protocol = val.(string)
		}

		// cast servers to string
		servers := vars["servers"].([]interface{})
		for _, s := range servers {
			// check if server have alias
			p := strings.Split(s.(string), " ")
			server := Server{
				IP: p[0],
			}
			if len(p) > 1 {
				server.Alias = p[1]
			}
			c.Servers = append(c.Servers, server)
			serversExists = true
		}
		// put back to map
		confs[key] = c
	}
	if !serversExists {
		return nil, ErrNoServersDetected
	}
	return confs, nil
}
