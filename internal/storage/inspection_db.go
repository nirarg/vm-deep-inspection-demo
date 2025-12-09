package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nirarg/v2v-vm-validations/pkg/persistent"
	pkgtypes "github.com/nirarg/v2v-vm-validations/pkg/types"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// VirtInspectorRecord represents a database record for VirtInspector inspection data
type VirtInspectorRecord struct {
	gorm.Model
	VMName       string `gorm:"index:idx_vm_snapshot,unique"`
	SnapshotName string `gorm:"index:idx_vm_snapshot,unique"`
	CacheKey     string `gorm:"uniqueIndex"`
	DataJSON     string `gorm:"type:longtext"` // MySQL: 4GB, PostgreSQL/SQLite: interpreted as TEXT
}

// VirtV2VInspectorRecord represents a database record for VirtV2vInspector inspection data
type VirtV2VInspectorRecord struct {
	gorm.Model
	VMName       string `gorm:"index:idx_vm_snapshot_v2v,unique"`
	SnapshotName string `gorm:"index:idx_vm_snapshot_v2v,unique"`
	CacheKey     string `gorm:"uniqueIndex"`
	DataJSON     string `gorm:"type:longtext"` // MySQL: 4GB, PostgreSQL/SQLite: interpreted as TEXT
}

// InspectionDB provides GORM-based persistent storage for inspection results
type InspectionDB struct {
	db     *gorm.DB
	logger *logrus.Logger
}

// NewInspectionDB creates a new GORM-based inspection database
func NewInspectionDB(db *gorm.DB, logger *logrus.Logger) (*InspectionDB, error) {
	// Auto-migrate the schema
	if err := db.AutoMigrate(&VirtInspectorRecord{}, &VirtV2VInspectorRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database schema: %w", err)
	}

	return &InspectionDB{
		db:     db,
		logger: logger,
	}, nil
}

// GetVirtInspectorXML retrieves VirtInspector inspection data for a given cache key
func (db *InspectionDB) GetVirtInspectorXML(ctx context.Context, key persistent.CacheKey) (*pkgtypes.VirtInspectorXML, error) {
	var record VirtInspectorRecord
	result := db.db.WithContext(ctx).Where("cache_key = ?", key.Hash()).First(&record)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Not found is not an error, just return nil
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query inspection data: %w", result.Error)
	}

	// Unmarshal JSON
	var data pkgtypes.VirtInspectorXML
	if err := json.Unmarshal([]byte(record.DataJSON), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inspection data: %w", err)
	}

	if db.logger != nil {
		db.logger.WithFields(logrus.Fields{
			"key":      key.String(),
			"cache_key": key.Hash(),
		}).Debug("Retrieved VirtInspector data from DB")
	}

	return &data, nil
}

// SetVirtInspectorXML stores VirtInspector inspection data for a given cache key
func (db *InspectionDB) SetVirtInspectorXML(ctx context.Context, key persistent.CacheKey, data *pkgtypes.VirtInspectorXML) error {
	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal inspection data: %w", err)
	}

	record := VirtInspectorRecord{
		VMName:       key.VMName,
		SnapshotName: key.SnapshotName,
		CacheKey:     key.Hash(),
		DataJSON:     string(jsonData),
	}

	// Use Create or update if exists
	result := db.db.WithContext(ctx).Where("cache_key = ?", key.Hash()).Assign(record).FirstOrCreate(&record)
	if result.Error != nil {
		return fmt.Errorf("failed to store inspection data: %w", result.Error)
	}

	if db.logger != nil {
		db.logger.WithFields(logrus.Fields{
			"key":      key.String(),
			"cache_key": key.Hash(),
		}).Debug("Stored VirtInspector data to DB")
	}

	return nil
}

// GetVirtV2VInspectorXML retrieves VirtV2vInspector inspection data for a given cache key
func (db *InspectionDB) GetVirtV2VInspectorXML(ctx context.Context, key persistent.CacheKey) (*pkgtypes.VirtV2VInspectorXML, error) {
	var record VirtV2VInspectorRecord
	result := db.db.WithContext(ctx).Where("cache_key = ?", key.Hash()).First(&record)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Not found is not an error, just return nil
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query inspection data: %w", result.Error)
	}

	// Unmarshal JSON
	var data pkgtypes.VirtV2VInspectorXML
	if err := json.Unmarshal([]byte(record.DataJSON), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inspection data: %w", err)
	}

	if db.logger != nil {
		db.logger.WithFields(logrus.Fields{
			"key":      key.String(),
			"cache_key": key.Hash(),
		}).Debug("Retrieved VirtV2VInspector data from DB")
	}

	return &data, nil
}

// SetVirtV2VInspectorXML stores VirtV2vInspector inspection data for a given cache key
func (db *InspectionDB) SetVirtV2VInspectorXML(ctx context.Context, key persistent.CacheKey, data *pkgtypes.VirtV2VInspectorXML) error {
	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal inspection data: %w", err)
	}

	record := VirtV2VInspectorRecord{
		VMName:       key.VMName,
		SnapshotName: key.SnapshotName,
		CacheKey:     key.Hash(),
		DataJSON:     string(jsonData),
	}

	// Use Create or update if exists
	result := db.db.WithContext(ctx).Where("cache_key = ?", key.Hash()).Assign(record).FirstOrCreate(&record)
	if result.Error != nil {
		return fmt.Errorf("failed to store inspection data: %w", result.Error)
	}

	if db.logger != nil {
		db.logger.WithFields(logrus.Fields{
			"key":      key.String(),
			"cache_key": key.Hash(),
		}).Debug("Stored VirtV2VInspector data to DB")
	}

	return nil
}
