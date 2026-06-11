package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OriginHealthStatus struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	RouteID    uint      `json:"route_id" gorm:"uniqueIndex:idx_origin_health_status_scope;index;not null;default:0"`
	NodeID     string    `json:"node_id" gorm:"uniqueIndex:idx_origin_health_status_scope;index;size:64;not null"`
	OriginURL  string    `json:"origin_url" gorm:"uniqueIndex:idx_origin_health_status_scope;size:2048;not null"`
	Status     string    `json:"status" gorm:"index;size:16;not null"`
	LatencyMS  int64     `json:"latency_ms" gorm:"not null;default:0"`
	LastError  string    `json:"last_error" gorm:"type:text"`
	ReportedAt time.Time `json:"reported_at" gorm:"index"`
	CheckedAt  time.Time `json:"checked_at" gorm:"index"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func UpsertOriginHealthStatus(tx *gorm.DB, status *OriginHealthStatus) error {
	if tx == nil {
		tx = DB
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "route_id"},
			{Name: "node_id"},
			{Name: "origin_url"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"status",
			"latency_ms",
			"last_error",
			"reported_at",
			"checked_at",
			"updated_at",
		}),
	}).Create(status).Error
}

func ListOriginHealthStatuses(routeID uint, nodeID string) (statuses []*OriginHealthStatus, err error) {
	query := DB.Order("reported_at desc").Order("id desc")
	if routeID > 0 {
		query = query.Where("route_id = ?", routeID)
	}
	if trimmedNodeID := strings.TrimSpace(nodeID); trimmedNodeID != "" {
		query = query.Where("node_id = ?", trimmedNodeID)
	}
	err = query.Find(&statuses).Error
	return statuses, err
}
