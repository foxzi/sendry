package models

import "time"

// TimeSeriesPoint represents a single point in the time series chart
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Sent      int       `json:"sent"`
	Failed    int       `json:"failed"`
	Pending   int       `json:"pending"`
}

// DomainStats represents statistics for a single sender domain
type DomainStats struct {
	Domain  string `json:"domain"`
	Sent    int    `json:"sent"`
	Failed  int    `json:"failed"`
	Pending int    `json:"pending"`
	Total   int    `json:"total"`
}

// MonitoringFilter defines filters for monitoring queries
type MonitoringFilter struct {
	Period string // "24h", "7d", "30d"
	Domain string // optional: filter by sender domain
}

// MonitoringData contains all data for the monitoring page charts
type MonitoringData struct {
	TimeSeries  []TimeSeriesPoint `json:"time_series"`
	DomainStats []DomainStats     `json:"domain_stats"`
	Period      string            `json:"period"`
	TotalSent   int               `json:"total_sent"`
	TotalFailed int               `json:"total_failed"`
	TotalQueue  int               `json:"total_queue"`
	TotalDLQ    int               `json:"total_dlq"`
}

// ServerStats represents live stats from a Sendry server
type ServerStats struct {
	Name      string `json:"name"`
	Env       string `json:"env"`
	Online    bool   `json:"online"`
	QueueSize int    `json:"queue_size"`
	DLQSize   int    `json:"dlq_size"`
	Error     string `json:"error,omitempty"`
}
