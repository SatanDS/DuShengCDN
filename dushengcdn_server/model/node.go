package model

import (
	"dushengcdn/utils/security"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Node struct {
	ID                        uint      `json:"id" gorm:"primaryKey"`
	NodeID                    string    `json:"node_id" gorm:"uniqueIndex;size:64;not null"`
	Name                      string    `json:"name" gorm:"size:128;not null"`
	IP                        string    `json:"ip" gorm:"size:64;not null"`
	PoolName                  string    `json:"pool_name" gorm:"size:64;not null;default:'default'"`
	Tags                      string    `json:"tags" gorm:"type:text;not null;default:'[]'"`
	Weight                    int       `json:"weight" gorm:"not null;default:100"`
	PublicIPs                 string    `json:"public_ips" gorm:"type:text;not null;default:'[]'"`
	SchedulingEnabled         bool      `json:"scheduling_enabled" gorm:"not null;default:true"`
	DrainMode                 bool      `json:"drain_mode" gorm:"not null;default:false"`
	GeoName                   string    `json:"geo_name" gorm:"size:128"`
	GeoLatitude               *float64  `json:"geo_latitude"`
	GeoLongitude              *float64  `json:"geo_longitude"`
	GeoManualOverride         bool      `json:"geo_manual_override" gorm:"not null;default:false"`
	AgentToken                string    `json:"-" gorm:"size:128;index"`
	AgentTokenHash            string    `json:"-" gorm:"column:agent_token_hash;size:71;index;not null;default:''"`
	AgentTokenPrefix          string    `json:"agent_token_prefix" gorm:"column:agent_token_prefix;size:16;index;not null;default:''"`
	AutoUpdateEnabled         bool      `json:"auto_update_enabled" gorm:"not null;default:false"`
	UpdateRequested           bool      `json:"update_requested" gorm:"not null;default:false"`
	UpdateChannel             string    `json:"update_channel" gorm:"size:16;not null;default:'stable'"`
	UpdateTag                 string    `json:"update_tag" gorm:"size:64"`
	RestartOpenrestyRequested bool      `json:"restart_openresty_requested" gorm:"not null;default:false"`
	AgentVersion              string    `json:"agent_version" gorm:"size:64;not null"`
	NginxVersion              string    `json:"nginx_version" gorm:"size:64"`
	OpenrestyStatus           string    `json:"openresty_status" gorm:"size:16;not null;default:'unknown'"`
	OpenrestyMessage          string    `json:"openresty_message" gorm:"type:text"`
	Status                    string    `json:"status" gorm:"size:16;not null;default:'offline'"`
	CurrentVersion            string    `json:"current_version" gorm:"size:32"`
	CurrentChecksum           string    `json:"current_checksum" gorm:"size:64;not null;default:''"`
	LastSeenAt                time.Time `json:"last_seen_at"`
	LastError                 string    `json:"last_error" gorm:"type:text"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

func ListNodes() (nodes []*Node, err error) {
	err = DB.Order("id desc").Find(&nodes).Error
	return nodes, err
}

func ListNodesByPool(poolName string) (nodes []*Node, err error) {
	poolName = strings.TrimSpace(poolName)
	if poolName == "" {
		poolName = "default"
	}
	err = DB.Where("pool_name = ?", poolName).Order("id desc").Find(&nodes).Error
	return nodes, err
}

func ListOnlineNodesByPool(poolName string, lastSeenAfter time.Time, connectedNodeIDs []string) (nodes []*Node, err error) {
	poolName = strings.TrimSpace(poolName)
	if poolName == "" {
		poolName = "default"
	}
	connectedNodeIDs = normalizeNodeIDs(connectedNodeIDs)
	query := DB.Where("pool_name = ?", poolName)
	switch {
	case !lastSeenAfter.IsZero() && len(connectedNodeIDs) > 0:
		query = query.Where("(last_seen_at >= ? OR node_id IN ?)", lastSeenAfter, connectedNodeIDs)
	case !lastSeenAfter.IsZero():
		query = query.Where("last_seen_at >= ?", lastSeenAfter)
	case len(connectedNodeIDs) > 0:
		query = query.Where("node_id IN ?", connectedNodeIDs)
	default:
		return []*Node{}, nil
	}
	err = query.Order("last_seen_at desc").Order("id desc").Find(&nodes).Error
	return nodes, err
}

func ListNodesByNodeIDs(nodeIDs []string) (nodes []*Node, err error) {
	nodeIDs = normalizeNodeIDs(nodeIDs)
	if len(nodeIDs) == 0 {
		return []*Node{}, nil
	}
	err = DB.Where("node_id IN ?", nodeIDs).Find(&nodes).Error
	return nodes, err
}

func normalizeNodeIDs(nodeIDs []string) []string {
	seen := make(map[string]struct{}, len(nodeIDs))
	normalized := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		normalized = append(normalized, nodeID)
	}
	return normalized
}

func GetNodeByNodeID(nodeID string) (*Node, error) {
	node := &Node{}
	err := DB.Where("node_id = ?", nodeID).First(node).Error
	return node, err
}

func GetNodeByID(id uint) (*Node, error) {
	node := &Node{}
	err := DB.First(node, id).Error
	return node, err
}

func GetNodeByAgentToken(token string) (*Node, error) {
	token = strings.TrimSpace(token)
	node := &Node{}
	tokenHash := security.HashSecretToken(token)
	if tokenHash != "" {
		err := DB.Where("agent_token_hash = ?", tokenHash).First(node).Error
		if err == nil {
			return node, nil
		}
		if err != gorm.ErrRecordNotFound {
			return node, err
		}
	}
	err := DB.Where("agent_token = ?", token).First(node).Error
	if err == nil && tokenHash != "" {
		if migrateErr := StoreNodeAgentTokenHash(node.ID, token); migrateErr == nil {
			node.AgentTokenHash = tokenHash
			node.AgentTokenPrefix = security.SecretTokenPrefix(token)
			node.AgentToken = ""
		}
	}
	return node, err
}

func StoreNodeAgentTokenHash(id uint, token string) error {
	return DB.Model(&Node{}).Where("id = ?", id).Updates(map[string]any{
		"agent_token":        "",
		"agent_token_hash":   security.HashSecretToken(token),
		"agent_token_prefix": security.SecretTokenPrefix(token),
	}).Error
}

func (node *Node) Insert() error {
	return node.InsertWithDB(DB)
}

func (node *Node) InsertWithDB(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	return db.Create(node).Error
}

func (node *Node) Update() error {
	return DB.Save(node).Error
}

func (node *Node) Delete() error {
	return DB.Delete(node).Error
}
