package model

import "time"

type GSLBSchedulingState struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	ProxyRouteID    uint       `json:"proxy_route_id" gorm:"uniqueIndex:idx_gslb_state_route_type_scope;not null"`
	DNSRecordType   string     `json:"dns_record_type" gorm:"uniqueIndex:idx_gslb_state_route_type_scope;size:16;not null;default:'A'"`
	ScopeKey        string     `json:"scope_key" gorm:"uniqueIndex:idx_gslb_state_route_type_scope;size:64;not null;default:'global'"`
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
	err := DB.Where("proxy_route_id = ? AND scope_key = ?", routeID, "global").First(state).Error
	return state, err
}

func GetGSLBSchedulingState(routeID uint, recordType string, scopeKey string) (*GSLBSchedulingState, error) {
	state := &GSLBSchedulingState{}
	err := DB.Where("proxy_route_id = ? AND dns_record_type = ? AND scope_key = ?", routeID, recordType, scopeKey).First(state).Error
	return state, err
}
