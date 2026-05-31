package model

import "time"

type GSLBSchedulingState struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	ProxyRouteID    uint       `json:"proxy_route_id" gorm:"uniqueIndex;not null"`
	DNSRecordType   string     `json:"dns_record_type" gorm:"size:16;not null;default:'A'"`
	SelectedTargets string     `json:"selected_targets" gorm:"type:text;not null;default:'[]'"`
	DesiredTargets  string     `json:"desired_targets" gorm:"type:text;not null;default:'[]'"`
	LastReason      string     `json:"last_reason" gorm:"type:text"`
	LastChangedAt   *time.Time `json:"last_changed_at"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func GetGSLBSchedulingStateByProxyRouteID(routeID uint) (*GSLBSchedulingState, error) {
	state := &GSLBSchedulingState{}
	err := DB.Where("proxy_route_id = ?", routeID).First(state).Error
	return state, err
}
