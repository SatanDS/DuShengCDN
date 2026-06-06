package model

import "time"

type CommercialLicenseActivation struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	ActivationID       string     `json:"activation_id" gorm:"size:128;uniqueIndex;not null;default:''"`
	LicenseID          string     `json:"license_id" gorm:"size:128;index;not null;default:''"`
	CustomerID         string     `json:"customer_id" gorm:"size:128;index;not null;default:''"`
	MachineFingerprint string     `json:"machine_fingerprint" gorm:"size:128;index;not null;default:''"`
	ServerVersion      string     `json:"server_version" gorm:"size:64;not null;default:''"`
	BuildWatermark     string     `json:"build_watermark" gorm:"size:128;not null;default:''"`
	InstanceHostname   string     `json:"instance_hostname" gorm:"size:255;not null;default:''"`
	RevokedAt          *time.Time `json:"revoked_at"`
	LastLeaseIssuedAt  *time.Time `json:"last_lease_issued_at"`
	LastLeaseExpiresAt *time.Time `json:"last_lease_expires_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (CommercialLicenseActivation) TableName() string {
	return "commercial_license_activations"
}
