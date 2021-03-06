/*
 * redis_exporter - scrapes redis stats and exports for prometheus.
 * Copyright (C) 2017 Joyield, Inc. <joyield.com@gmail.com>
 * All rights reserved.
 */
package exporter

import (
	"github.com/garyburd/redigo/redis"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	namespace = "redis"
)

var (
	globalGauges = [][]string{
		{"used_memory", "Current alloc memory"},
		{"used_cpu", "Used cpu"},
		{"total_commands_processed", "Total commands processed"},
		{"total_net_input_bytes", "Total net input bytes"},
		{"total_net_output_bytes", "Total net output bytes"},
	}
	clusterGauges = [][]string{
		{"used_memory", "Current alloc memory"},
		{"master_used_memory", "Current alloc memory"},
		{"used_memory_rss", "Used memory rss"},
		{"used_memory_peak", "Used memory peak"},
		{"used_memory_lua", "Used memory lua"},
		{"maxmemory", "Max memory"},
		{"master_maxmemory", "Max memory"},
		{"used_cpu_sys", "Used cpu sys"},
		{"used_cpu_user", "Used cpu user"},
		{"used_cpu", "Used cpu"},
		{"total_connections_received", "Total connections received"},
		{"connected_clients", "Current client connections"},
		{"blocked_clients", "Blocked clients"},
		{"rejected_connections", "Rejected connections"},
		{"total_commands_processed", "Total commands processed"},
		{"total_net_input_bytes", "Total net input bytes"},
		{"total_net_output_bytes", "Total net output bytes"},
		{"sync_full", "Sync full"},
		{"sync_partial_ok", "sync_partial_ok"},
		{"sync_partial_err", "sync_partial_err"},
		{"expired_keys", "expired_keys"},
		{"evicted_keys", "evicted_keys"},
		{"keyspace_hits", "keyspace_hits"},
		{"keyspace_misses", "keyspace_misses"},
		{"pubsub_channels", "pubsub_channels"},
		{"pubsub_patterns", "pubsub_patterns"},
	}
)

type Exporter struct {
	mutex         sync.RWMutex
	addr          string
	password      string
	name          string
	globalGauges  map[string]prometheus.Gauge
	clusterGauges map[string]prometheus.Gauge
	cmdstat       *prometheus.GaugeVec
	dbkeys        *prometheus.GaugeVec
	dbexpires     *prometheus.GaugeVec
	conn          redis.Conn
}

func NewExporter(uri, name string) (*Exporter, error) {
	addr := uri
	password := ""
	idx := strings.LastIndex(uri, "@")
	if idx >= 0 {
		password = uri[:idx]
		addr = uri[idx+1:]
	}
	e := &Exporter{
		addr:          addr,
		password:      password,
		name:          name,
		globalGauges:  map[string]prometheus.Gauge{},
		clusterGauges: map[string]prometheus.Gauge{},
		cmdstat: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   name,
			Name:        "cmdstat",
			Help:        "Commands stat",
			ConstLabels: prometheus.Labels{"addr": addr},
		}, []string{"cmd"}),
		dbkeys: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   name,
			Name:        "dbkeys",
			Help:        "Database key count",
			ConstLabels: prometheus.Labels{"addr": addr},
		}, []string{"db", "role"}),
		dbexpires: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   name,
			Name:        "dbexpires",
			Help:        "Database expire key count",
			ConstLabels: prometheus.Labels{"addr": addr},
		}, []string{"db", "role"}),
	}
	for _, m := range globalGauges {
		e.globalGauges[m[0]] = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        m[0],
			Help:        m[1],
			ConstLabels: prometheus.Labels{"cluster": name, "addr": addr},
		})
	}
	for _, m := range clusterGauges {
		e.clusterGauges[m[0]] = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   name,
			Name:        m[0],
			Help:        m[1],
			ConstLabels: prometheus.Labels{"addr": addr},
		})
	}
	return e, nil
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, g := range e.globalGauges {
		ch <- g.Desc()
	}
	for _, g := range e.clusterGauges {
		ch <- g.Desc()
	}
	e.cmdstat.Describe(ch)
	e.dbkeys.Describe(ch)
	e.dbexpires.Describe(ch)
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.resetMetrics()
	err := e.scrape()
	if err != nil {
		return
	}
	for _, g := range e.globalGauges {
		ch <- g
	}
	for _, g := range e.clusterGauges {
		ch <- g
	}
	e.cmdstat.Collect(ch)
	e.dbkeys.Collect(ch)
	e.dbexpires.Collect(ch)
}

func (e *Exporter) resetMetrics() {
	e.cmdstat.Reset()
	e.dbkeys.Reset()
	e.dbexpires.Reset()
}

func (e *Exporter) scrape() error {
	var err error
	c := e.conn
	if c == nil {
		timeout := 5 * time.Second
		c, err = redis.Dial(
			"tcp",
			e.addr,
			redis.DialConnectTimeout(timeout),
			redis.DialReadTimeout(timeout),
			redis.DialWriteTimeout(timeout),
		)
		if err != nil {
			log.Printf("dial redis %s err:%q\n", e.addr, err)
			return err
		}
		e.conn = c
	}
	if len(e.password) > 0 {
		c.Do("AUTH", e.password)
	}
	r, err := redis.String(c.Do("INFO", "all"))
	if err != nil {
		log.Printf("redis %s do INFO err:%q\n", e.addr, err)
		c.Close()
		e.conn = nil
		return err
	}
	role := "none"
	if strings.Index(r, "role:master") >= 0 {
		role = "master"
	} else if strings.Index(r, "role:slave") >= 0 {
		role = "slave"
	}
	cpu := 0.
	used_memory := 0.
	maxmemory := 0.
	lines := strings.Split(r, "\r\n")
	for _, line := range lines {
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		k := line[:idx]
		v := line[idx+1:]
		if strings.HasPrefix(k, "cmdstat_") {
			cmd := k[8:]
			b := strings.Index(v, "=")
			t := strings.Index(v, ",")
			if b > 0 && b < t {
				cnt, err := strconv.ParseFloat(v[b+1:t], 64)
				if err == nil {
					e.cmdstat.WithLabelValues(cmd).Set(cnt)
				}
			}
		} else if strings.HasPrefix(k, "db") {
			if len(k) >= 3 {
				db := k[2:]
				items := strings.Split(v, ",")
				for _, s := range items {
					if strings.HasPrefix(s, "keys=") {
						cnt, err := strconv.ParseFloat(s[5:], 64)
						if err == nil {
							e.dbkeys.WithLabelValues(db, role).Set(cnt)
						}
					} else if strings.HasPrefix(s, "expires=") {
						cnt, err := strconv.ParseFloat(s[8:], 64)
						if err == nil {
							e.dbexpires.WithLabelValues(db, role).Set(cnt)
						}
					}
				}
			}
		} else {
			g0, ok0 := e.globalGauges[k]
			g1, ok1 := e.clusterGauges[k]
			if ok0 || ok1 {
				val, err := strconv.ParseFloat(v, 64)
				if err == nil {
					if ok0 {
						g0.Set(val)
					}
					if ok1 {
						g1.Set(val)
					}
					if k == "used_memory" {
						used_memory = val
					} else if k == "maxmemory" {
						maxmemory = val
					} else if k == "used_cpu_sys" || k == "used_cpu_user" {
						cpu += val
					}
				}
			}
		}
	}
	if role != "master" {
		used_memory = 0.
		maxmemory = 0.
	}
	if g, ok := e.clusterGauges["master_used_memory"]; ok {
		g.Set(used_memory)
	}
	if g, ok := e.clusterGauges["master_maxmemory"]; ok {
		g.Set(maxmemory)
	}
	if g, ok := e.globalGauges["used_cpu"]; ok {
		g.Set(cpu)
	}
	if g, ok := e.clusterGauges["used_cpu"]; ok {
		g.Set(cpu)
	}
	return nil
}
