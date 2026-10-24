package app

import (
	"encoding/json"
	"math"
	"time"
)

type expiryInfo struct {
	State            string  `json:"state"`
	NotBefore        string  `json:"not_before,omitempty"`
	NotAfter         string  `json:"not_after,omitempty"`
	DaysRemaining    int     `json:"days_remaining"`
	RemainingPercent float64 `json:"remaining_percent"`
	WarningDays      int     `json:"warning_days"`
	Total            int     `json:"total"`
	Known            int     `json:"known"`
	NotDeployed      int     `json:"not_deployed"`
	Unknown          int     `json:"unknown"`
}

func certificateExpiry(active any, raw any, warningDays int, now time.Time) expiryInfo {
	info := expiryInfo{State: "Unknown", WarningDays: warningDays, Total: 1, Unknown: 1}
	if active == nil || active == "" {
		info.State = "Not deployed"
		info.NotDeployed = 1
		info.Unknown = 0
		return info
	}
	text, ok := raw.(string)
	if !ok {
		return info
	}
	var meta struct {
		NotBefore time.Time `json:"not_before"`
		NotAfter  time.Time `json:"not_after"`
	}
	if json.Unmarshal([]byte(text), &meta) != nil || meta.NotBefore.IsZero() || meta.NotAfter.IsZero() || !meta.NotAfter.After(meta.NotBefore) {
		return info
	}
	info.NotBefore = meta.NotBefore.UTC().Format(time.RFC3339)
	info.NotAfter = meta.NotAfter.UTC().Format(time.RFC3339)
	info.DaysRemaining = int(math.Ceil(meta.NotAfter.Sub(now).Hours() / 24))
	info.RemainingPercent = math.Max(0, math.Min(100, 100*float64(meta.NotAfter.Sub(now))/float64(meta.NotAfter.Sub(meta.NotBefore))))
	info.Known = 1
	info.Unknown = 0
	info.State = "Healthy"
	if !meta.NotAfter.After(now) {
		info.State = "Expired"
	} else if meta.NotAfter.Sub(now) <= time.Duration(warningDays)*24*time.Hour {
		info.State = "Warning"
	}
	return info
}

func summarizeExpiry(targets []map[string]any, warningDays int, now time.Time) map[string]expiryInfo {
	summaries := map[string]expiryInfo{}
	for _, t := range targets {
		server := t["server_id"].(string)
		item := certificateExpiry(t["active_version"], t["active_metadata"], warningDays, now)
		t["expiry"] = item
		summary := summaries[server]
		if item.Known > 0 && (summary.NotAfter == "" || item.NotAfter < summary.NotAfter) {
			summary.State = item.State
			summary.NotBefore = item.NotBefore
			summary.NotAfter = item.NotAfter
			summary.DaysRemaining = item.DaysRemaining
			summary.RemainingPercent = item.RemainingPercent
		}
		summary.Total++
		summary.Known += item.Known
		summary.Unknown += item.Unknown
		summary.NotDeployed += item.NotDeployed
		summary.WarningDays = warningDays
		if summary.Known == 0 {
			summary.State = "Not deployed"
			if summary.Unknown > 0 {
				summary.State = "Unknown"
			}
		}
		summaries[server] = summary
	}
	return summaries
}
