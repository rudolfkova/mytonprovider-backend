// Package lokipush sends compact RunChecks summaries to Grafana Loki via HTTP push API.
package lokipush

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
)

type pushRequest struct {
	Streams []streamEntry `json:"streams"`
}

type streamEntry struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// PushRunChecks sends one "job" row and one "ip" row per distinct storage IP (best-effort).
func PushRunChecks(
	ctx context.Context,
	baseURL string,
	client *http.Client,
	jobID, agentID, location string,
	finishedUnix int64,
	durationMs int64,
	results []*providerchecksv1.ContractCheckResult,
	providers []*providerchecksv1.ProviderBatch,
) error {
	if baseURL == "" {
		return nil
	}
	if client == nil {
		client = http.DefaultClient
	}

	pubToIP := make(map[string]string)
	for _, p := range providers {
		if p == nil || p.GetStorageEndpoint() == nil {
			continue
		}
		ip := p.GetStorageEndpoint().GetIp()
		if ip == "" {
			continue
		}
		pubToIP[p.GetProviderPubkey()] = ip
	}

	reasonTotals := make(map[string]int)
	ipReason := make(map[string]map[string]int)
	validTotal := 0
	total := 0

	for _, r := range results {
		if r == nil {
			continue
		}
		total++
		rc := r.GetReasonCode().String()
		reasonTotals[rc]++

		ip := pubToIP[r.GetProviderPubkey()]
		if ip == "" {
			ip = "unknown"
		}
		if ipReason[ip] == nil {
			ipReason[ip] = make(map[string]int)
		}
		ipReason[ip][rc]++
		if r.GetReasonCode() == providerchecksv1.ReasonCode_VALID_STORAGE_PROOF {
			validTotal++
		}
	}

	ts := strconv.FormatInt(time.Now().UnixNano(), 10)

	jobLine := buildFlatJSON(jobID, agentID, location, finishedUnix, durationMs, total, validTotal, total-validTotal, reasonTotals)

	streams := []streamEntry{
		{
			Stream: map[string]string{
				"job":      "runchecks",
				"event":    "job",
				"job_id":   jobID,
				"agent_id": agentID,
			},
			Values: [][]string{{ts, jobLine}},
		},
	}

	for ip, counts := range ipReason {
		v, inv := countValidInvalid(counts)
		line := buildFlatJSON(jobID, agentID, location, finishedUnix, durationMs, v+inv, v, inv, counts)
		streams = append(streams, streamEntry{
			Stream: map[string]string{
				"job":        "runchecks",
				"event":      "ip",
				"job_id":     jobID,
				"storage_ip": ip,
				"agent_id":   agentID,
			},
			Values: [][]string{{ts, line}},
		})
	}

	body, err := json.Marshal(pushRequest{Streams: streams})
	if err != nil {
		return fmt.Errorf("marshal loki push: %w", err)
	}

	pushURL := baseURL + "/loki/api/v1/push"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pushURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("loki request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("loki push: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("loki push: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func countValidInvalid(byReason map[string]int) (valid, invalid int) {
	validKey := providerchecksv1.ReasonCode_VALID_STORAGE_PROOF.String()
	for k, n := range byReason {
		if k == validKey {
			valid += n
		} else {
			invalid += n
		}
	}
	return valid, invalid
}

func buildFlatJSON(jobID, agentID, location string, finishedUnix, durationMs int64, total, valid, invalid int, byReason map[string]int) string {
	m := map[string]interface{}{
		"job_id":        jobID,
		"agent_id":      agentID,
		"location":      location,
		"finished_unix": finishedUnix,
		"duration_ms":   durationMs,
		"total":         total,
		"valid":         valid,
		"invalid":       invalid,
	}
	for _, name := range providerchecksv1.ReasonCode_name {
		k := "n_" + name
		if v, ok := byReason[name]; ok {
			m[k] = v
		} else {
			m[k] = 0
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return `{}`
	}
	return string(b)
}
