/*
 * redis_exporter - scrapes redis stats and exports for prometheus.
 * Copyright (C) 2017 Joyield, Inc. <joyield.com@gmail.com>
 * All rights reserved.
 */
package main

import (
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"redis_exporter/exporter"
)

func main() {
	var (
		bind = flag.String("bind", ":9379", "Listen address")
		addr = flag.String("redis", "127.0.0.1:6379", "Redis service address")
		name = flag.String("name", "none", "Redis service name")
	)
	flag.Parse()
	exporter, err := exporter.NewExporter(*addr, *name)
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(exporter)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*bind, nil))
}
