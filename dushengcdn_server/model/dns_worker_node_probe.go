package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DNSWorkerNodeProbe struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	WorkerID       string    `json:"worker_id" gorm:"uniqueIndex:idx_dns_worker_node_probe;size:64;not null"`
	NodeID         string    `json:"node_id" gorm:"uniqueIndex:idx_dns_worker_node_probe;size:64;not null"`
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
