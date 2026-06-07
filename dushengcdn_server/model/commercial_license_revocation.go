package model

import "time"

type CommercialLicenseRevocation struct {
	ID         uint       `json:"id" gorm:"column:id;primaryKey"`
	LicenseID  string     `json:"license_id" gorm:"column:license_id;size:128;uniqueIndex;not null;default:''"`
	CustomerID string     `json:"customer_id" gorm:"column:customer_id;size:128;index;not null;default:''"`
	Reason     string     `json:"reason" gorm:"column:reason;size:255;not null;default:''"`
	RevokedAt  *time.Time `json:"revoked_at" gorm:"column:revoked_at"`
	CreatedAt  time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt  time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (CommercialLicenseRevocation) TableName() string {
	return "commercial_license_revocations"
}
