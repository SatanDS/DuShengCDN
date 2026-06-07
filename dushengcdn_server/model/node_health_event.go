package model

import "time"

type NodeHealthEvent struct {
	ID               uint       `json:"id" gorm:"column:id;primaryKey"`
	NodeID           string     `json:"node_id" gorm:"column:node_id;index;size:64;not null"`
	EventType        string     `json:"event_type" gorm:"column:event_type;index;size:64;not null"`
	Severity         string     `json:"severity" gorm:"column:severity;size:16;not null"`
	Status           string     `json:"status" gorm:"column:status;index;size:16;not null"`
	Message          string     `json:"message" gorm:"column:message;type:text"`
	FirstTriggeredAt time.Time  `json:"first_triggered_at" gorm:"column:first_triggered_at;index"`
	LastTriggeredAt  time.Time  `json:"last_triggered_at" gorm:"column:last_triggered_at;index"`
	ReportedAt       time.Time  `json:"reported_at" gorm:"column:reported_at;index"`
	ResolvedAt       *time.Time `json:"resolved_at" gorm:"column:resolved_at;index"`
	MetadataJSON     string     `json:"metadata_json" gorm:"column:metadata_json;type:text"`
	CreatedAt        time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt        time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func GetActiveNodeHealthEvent(nodeID string, eventType string) (*NodeHealthEvent, error) {
	event := &NodeHealthEvent{}
	err := DB.Where("node_id = ? AND event_type = ? AND status = ?", nodeID, eventType, "active").First(event).Error
	return event, err
}

func ListNodeHealthEvents(nodeID string, activeOnly bool, limit int) (events []*NodeHealthEvent, err error) {
	query := DB.Where("node_id = ?", nodeID).Order("last_triggered_at desc")
	if activeOnly {
		query = query.Where("status = ?", "active")
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	err = query.Find(&events).Error
	return events, err
}

func ListActiveNodeHealthEvents() (events []*NodeHealthEvent, err error) {
	err = DB.Where("status = ?", "active").Order("last_triggered_at desc").Find(&events).Error
	return events, err
}

func DeleteNodeHealthEvents(nodeID string) (deleted int64, err error) {
	result := DB.Where("node_id = ?", nodeID).Delete(&NodeHealthEvent{})
	return result.RowsAffected, result.Error
}
