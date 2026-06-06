package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const commercialLicenseSingletonID uint = 1

type CommercialLicense struct {
	ID                  uint       `json:"id" gorm:"primaryKey"`
	LicenseID           string     `json:"license_id" gorm:"size:128;index;not null;default:''"`
	CustomerID          string     `json:"customer_id" gorm:"size:128;not null;default:''"`
	CustomerName        string     `json:"customer_name" gorm:"size:255;not null;default:''"`
	Plan                string     `json:"plan" gorm:"size:64;not null;default:''"`
	Status              string     `json:"status" gorm:"size:32;index;not null;default:''"`
	Token               string     `json:"-" gorm:"type:text;not null"`
	TokenHash           string     `json:"token_hash" gorm:"size:64;not null;default:''"`
	Fingerprint         string     `json:"fingerprint" gorm:"size:32;not null;default:''"`
	FeaturesJSON        string     `json:"features_json" gorm:"type:text;not null;default:'[]'"`
	MaxNodes            int        `json:"max_nodes" gorm:"not null;default:0"`
	MaxSites            int        `json:"max_sites" gorm:"not null;default:0"`
	IssuedAt            *time.Time `json:"issued_at"`
	ExpiresAt           *time.Time `json:"expires_at"`
	ActivationID        string     `json:"activation_id" gorm:"size:128;index;not null;default:''"`
	MachineFingerprint  string     `json:"machine_fingerprint" gorm:"size:128;not null;default:''"`
	LeaseToken          string     `json:"-" gorm:"type:text;not null;default:''"`
	LeaseExpiresAt      *time.Time `json:"lease_expires_at"`
	LastLeaseRenewedAt  *time.Time `json:"last_lease_renewed_at"`
	LastValidatedAt     *time.Time `json:"last_validated_at"`
	LastValidationError string     `json:"last_validation_error" gorm:"type:text;not null;default:''"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (CommercialLicense) TableName() string {
	return "commercial_licenses"
}

func GetCommercialLicense() (*CommercialLicense, error) {
	license := &CommercialLicense{}
	err := DB.Where("id = ?", commercialLicenseSingletonID).First(license).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return license, nil
}

func SaveCommercialLicense(license *CommercialLicense) error {
	if license == nil {
		return errors.New("license is nil")
	}
	license.ID = commercialLicenseSingletonID
	license.Token = strings.TrimSpace(license.Token)
	license.TokenHash = CommercialLicenseTokenHash(license.Token)
	if len(license.TokenHash) >= 16 {
		license.Fingerprint = license.TokenHash[:16]
	}
	return DB.Save(license).Error
}

func DeleteCommercialLicense() error {
	return DB.Delete(&CommercialLicense{}, commercialLicenseSingletonID).Error
}

func CommercialLicenseTokenHash(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
