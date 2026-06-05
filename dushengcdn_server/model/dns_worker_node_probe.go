package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DNSWorkerNodeProbe struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	WorkerID       string    `json:"worker_id" gorm:"uniqueIndex:idx_dns_worker_node_probe;size:64;not null"`
	NodeID         string    `json:"node_id" gorm:"uniqueIndex:idx_dns_worker_node_probe;index:idx_dns_worker_node_probe_node;size:64;not null"`
	PublicAddress  string    `json:"public_address" gorm:"size:255;not null;default:''"`
	QueryName      string    `json:"query_name" gorm:"size:255;not null;default:''"`
	QueryType      string    `json:"query_type" gorm:"size:16;not null;default:'SOA'"`
	CheckedAt      time.Time `json:"checked_at" gorm:"index"`
	ResultsJSON    string    `json:"results_json" gorm:"type:text;not null;default:'[]'"`
	Healthy        bool      `json:"healthy" gorm:"index;not null;default:false"`
	AverageRTTMs   float64   `json:"average_rtt_ms" gorm:"not null;default:0"`
	MaxRTTMs       int64     `json:"max_rtt_ms" gorm:"not null;default:0"`
	LastError      string    `json:"last_error" gorm:"type:text"`
	FailureSamples int       `json:"failure_samples" gorm:"not null;default:0"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func UpsertDNSWorkerNodeProbe(tx *gorm.DB, probe *DNSWorkerNodeProbe) error {
	if tx == nil {
		tx = DB
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "worker_id"},
			{Name: "node_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"public_address",
			"query_name",
			"query_type",
			"checked_at",
			"results_json",
			"healthy",
			"average_rtt_ms",
			"max_rtt_ms",
			"last_error",
			"failure_samples",
			"updated_at",
		}),
	}).Create(probe).Error
}

func ListDNSWorkerNodeProbes() (probes []*DNSWorkerNodeProbe, err error) {
	err = DB.Order("checked_at desc").Find(&probes).Error
	return probes, err
}

func ListDNSWorkerNodeProbesByScope(workerIDs []string, nodeIDs []string) (probes []*DNSWorkerNodeProbe, err error) {
	workerIDs = normalizeUniqueStrings(workerIDs)
	nodeIDs = normalizeUniqueStrings(nodeIDs)
	if len(workerIDs) == 0 || len(nodeIDs) == 0 {
		return []*DNSWorkerNodeProbe{}, nil
	}
	err = DB.Where("worker_id IN ? AND node_id IN ?", workerIDs, nodeIDs).
		Order("worker_id asc").
		Order("node_id asc").
		Find(&probes).Error
	return probes, err
}

func normalizeUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
