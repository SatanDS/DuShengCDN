package configversion

import "dushengcdn/model"

type VersionService interface {
	ListConfigVersions() ([]*model.ConfigVersionSummary, error)
	GetConfigVersionDetail(id uint) (any, error)
	GetActiveConfigVersion() (any, error)
	PreviewConfigVersion() (any, error)
	DiffConfigVersion() (any, error)
	HasConfigChanges() (bool, error)
	PublishConfigVersion(createdBy string, force bool) (any, error)
	ActivateConfigVersion(id uint) (*model.ConfigVersion, error)
	CleanupConfigVersions(keepCount int) (int64, error)
}

type Facade struct {
	Service VersionService
}

func (f Facade) ListConfigVersions() ([]*model.ConfigVersionSummary, error) {
	return f.Service.ListConfigVersions()
}

func (f Facade) GetConfigVersionDetail(id uint) (any, error) {
	return f.Service.GetConfigVersionDetail(id)
}

func (f Facade) GetActiveConfigVersion() (any, error) {
	return f.Service.GetActiveConfigVersion()
}

func (f Facade) PreviewConfigVersion() (any, error) {
	return f.Service.PreviewConfigVersion()
}

func (f Facade) DiffConfigVersion() (any, error) {
	return f.Service.DiffConfigVersion()
}

func (f Facade) HasConfigChanges() (bool, error) {
	return f.Service.HasConfigChanges()
}

func (f Facade) PublishConfigVersion(createdBy string, force bool) (any, error) {
	return f.Service.PublishConfigVersion(createdBy, force)
}

func (f Facade) ActivateConfigVersion(id uint) (*model.ConfigVersion, error) {
	return f.Service.ActivateConfigVersion(id)
}

func (f Facade) CleanupConfigVersions(keepCount int) (int64, error) {
	return f.Service.CleanupConfigVersions(keepCount)
}
