package main

import (
	"io/ioutil"
	"log"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	_, err := LoadConfig("files/nutcracker.yml")
	if err != nil {
		t.Error("Failed to read config: ", err.Error())
	}
}

func TestLoadMetrics(t *testing.T) {
	conf, err := LoadConfig("files/nutcracker.yml")
	if err != nil {
		t.Error("Failed to read config: ", err.Error())
	}

	resp, err := ioutil.ReadFile("files/example.json")
	if err != nil {
		t.Error("Failed to read json example: ", err.Error())
	}

	stats, err := parseStats(resp, conf)
	if err != nil {
		t.Error("Failed to parse stats: ", err.Error())
	}
	log.Printf("Stats: %+v", stats)
}
