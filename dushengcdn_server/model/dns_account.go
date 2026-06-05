package model

import "time"

type DnsAccount struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	Name          string    `json:"name" gorm:"size:255;not null"`
	Type          string    `json:"type" gorm:"size:64;not null"`
	Authorization string    `json:"-" gorm:"type:text;not null"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func ListDnsAccounts() (accounts []*DnsAccount, err error) {
	err = DB.Order("id desc").Find(&accounts).Error
	return accounts, err
}

func GetDnsAccountByID(id uint) (*DnsAccount, error) {
	account := &DnsAccount{}
	err := DB.First(account, id).Error
	return account, err
}

func ListDnsAccountsByIDs(ids []uint) (accounts []*DnsAccount, err error) {
	if len(ids) == 0 {
		return []*DnsAccount{}, nil
	}
	err = DB.Where("id IN ?", ids).Find(&accounts).Error
	return accounts, err
}

func (account *DnsAccount) Insert() error {
	return DB.Create(account).Error
}

func (account *DnsAccount) Update() error {
	return DB.Save(account).Error
}

func (account *DnsAccount) Delete() error {
	return DB.Delete(account).Error
}
