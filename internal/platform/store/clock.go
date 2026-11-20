package store

import "gorm.io/gorm"

// errRecordNotFound is GORM's own error for a row that is not there.
var errRecordNotFound = gorm.ErrRecordNotFound
