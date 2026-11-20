package store

import (
	"time"

	"gorm.io/gorm"
)

// DataImport is the record of the import of the state one entity kept in files (see RunImports): when it ran and what it did.
type DataImport struct {
	Entity     string `gorm:"primaryKey;size:191"`
	State      string `gorm:"size:32;not null"` // running, done or failed
	Created    int
	Existing   int
	Skipped    Text // JSON: [{path, reason}]
	Error      Text
	StartedAt  time.Time
	FinishedAt *time.Time
}

// TableName is the table of the records.
func (DataImport) TableName() string { return "data_imports" }

// dataImportsMigration creates the table of the records of the imports.
func dataImportsMigration() *Migration {
	return &Migration{
		ID: "202611181210_data_imports",
		Migrate: func(tx *gorm.DB) error {
			// The struct is copied here: the migration must do the same whatever DataImport becomes.
			type dataImport struct {
				Entity     string `gorm:"primaryKey;size:191"`
				State      string `gorm:"size:32;not null"`
				Created    int
				Existing   int
				Skipped    Text
				Error      Text
				StartedAt  time.Time
				FinishedAt *time.Time
			}
			if tx.Migrator().HasTable("data_imports") {
				return nil
			}
			return tx.Table("data_imports").Migrator().CreateTable(&dataImport{})
		},
		Rollback: func(tx *gorm.DB) error { return tx.Migrator().DropTable("data_imports") },
	}
}
