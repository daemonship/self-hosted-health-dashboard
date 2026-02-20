// Package main is the health-dashboard agent binary.
// It collects system metrics (CPU, memory, disk) every 30s and POSTs them
// to the server's POST /api/metrics endpoint using a shared token.
//
// Supported platforms: Linux (reads /proc/stat, /proc/meminfo, /proc/mounts).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"health-dashboard/internal/config"
)

// diskStat holds space usage for a single mount point.
type diskStat struct {
	Mount string `json:"mount"`
	Used  int64  `json:"used"`
	Total int64  `json:"total"`
}

// metricsPayload is the JSON body sent to POST /api/metrics.
type metricsPayload struct {
	CPUPercent float64    `json:"cpu_percent"`
	MemUsed    int64      `json:"mem_used"`
	MemTotal   int64      `json:"mem_total"`
	Disks      []diskStat `json:"disks"`
}

// cpuSample holds raw jiffies from a single /proc/stat reading.
type cpuSample struct {
	total int64
	idle  int64
}

// readCPUSample reads the aggregate "cpu" line from /proc/stat.
func readCPUSample() (cpuSample, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		// cpu  user nice system idle iowait irq softirq steal ...
		fields := strings.Fields(line)
		if len(fields) < 8 {
			break
		}
		var v [8]int64
		for i := range v {
			v[i], _ = strconv.ParseInt(fields[i+1], 10, 64)
		}
		total := v[0] + v[1] + v[2] + v[3] + v[4] + v[5] + v[6] + v[7]
		idle := v[3] + v[4] // idle + iowait
		return cpuSample{total: total, idle: idle}, nil
	}
	return cpuSample{}, fmt.Errorf("cpu line not found in /proc/stat")
}

// cpuPercentBetween calculates the CPU usage percentage between two samples.
func cpuPercentBetween(a, b cpuSample) float64 {
	totalDelta := b.total - a.total
	idleDelta := b.idle - a.idle
	if totalDelta <= 0 {
		return 0
	}
	pct := 100.0 * float64(totalDelta-idleDelta) / float64(totalDelta)
	if pct < 0 {
		return 0
	}
	return pct
}

// readMemInfo returns (used, total) bytes from /proc/meminfo.
// used = MemTotal - MemAvailable.
func readMemInfo() (used, total int64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	var memTotal, memAvailable int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		// Values are in kB; convert to bytes.
		val, _ := strconv.ParseInt(fields[1], 10, 64)
		val *= 1024
		switch fields[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		}
	}
	return memTotal - memAvailable, memTotal, nil
}

// virtualFSTypes is the set of filesystem types we skip when collecting disk stats.
var virtualFSTypes = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "sysfs": true, "proc": true,
	"cgroup": true, "cgroup2": true, "devpts": true, "hugetlbfs": true,
	"mqueue": true, "pstore": true, "securityfs": true, "debugfs": true,
	"tracefs": true, "bpf": true, "overlay": true, "fusectl": true,
	"squashfs": true, "nsfs": true, "efivarfs": true,
}

// readDiskStats returns used/total bytes for each real mounted filesystem
// by reading /proc/mounts and calling Statfs on each mount point.
func readDiskStats() ([]diskStat, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[string]bool)
	var disks []diskStat

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		// Format: device mountpoint fstype options dump pass
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mount := fields[1]
		fstype := fields[2]

		if virtualFSTypes[fstype] {
			continue
		}
		if seen[mount] {
			continue
		}
		seen[mount] = true

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			continue // inaccessible mount — skip silently
		}
		if stat.Blocks == 0 {
			continue
		}
		total := int64(stat.Blocks) * stat.Bsize
		used := int64(stat.Blocks-stat.Bfree) * stat.Bsize
		disks = append(disks, diskStat{Mount: mount, Used: used, Total: total})
	}
	return disks, nil
}

// collect gathers a full metrics snapshot.
// CPU sampling takes ~1s (two /proc/stat reads with a 1s sleep between them).
func collect() (metricsPayload, error) {
	s1, err := readCPUSample()
	if err != nil {
		return metricsPayload{}, fmt.Errorf("cpu sample 1: %w", err)
	}
	time.Sleep(time.Second)
	s2, err := readCPUSample()
	if err != nil {
		return metricsPayload{}, fmt.Errorf("cpu sample 2: %w", err)
	}

	memUsed, memTotal, err := readMemInfo()
	if err != nil {
		return metricsPayload{}, fmt.Errorf("meminfo: %w", err)
	}

	disks, err := readDiskStats()
	if err != nil {
		return metricsPayload{}, fmt.Errorf("diskstats: %w", err)
	}

	return metricsPayload{
		CPUPercent: cpuPercentBetween(s1, s2),
		MemUsed:    memUsed,
		MemTotal:   memTotal,
		Disks:      disks,
	}, nil
}

// send POSTs the metrics payload to the server.
func send(client *http.Client, serverURL, token string, payload metricsPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/metrics", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func run(client *http.Client, serverURL, token string) {
	payload, err := collect()
	if err != nil {
		log.Printf("agent: collect error: %v", err)
		return
	}
	if err := send(client, serverURL, token, payload); err != nil {
		log.Printf("agent: send error: %v", err)
		return
	}
	log.Printf("agent: sent cpu=%.1f%% mem=%d/%d disks=%d",
		payload.CPUPercent, payload.MemUsed, payload.MemTotal, len(payload.Disks))
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("agent: config: %v", err)
	}
	if cfg.Agent.Token == "" {
		log.Fatal("agent: agent.token must be set in config.yaml")
	}
	if cfg.Agent.ServerURL == "" {
		log.Fatal("agent: agent.server_url must be set in config.yaml")
	}

	log.Printf("agent: reporting to %s every 30s", cfg.Agent.ServerURL)

	client := &http.Client{Timeout: 10 * time.Second}

	// Send once immediately on startup, then tick every 30s.
	// collect() takes ~1s for the CPU sample, so the effective interval
	// between reports is ~30s.
	run(client, cfg.Agent.ServerURL, cfg.Agent.Token)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		run(client, cfg.Agent.ServerURL, cfg.Agent.Token)
	}
}
