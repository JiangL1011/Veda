package app

import (
	"veda/internal/model"
	"veda/internal/store"
)

// GetDataMigrationState 返回启动迁移的当前状态，供前端轮询。
func (s *AppService) GetDataMigrationState() *model.DataMigrationState {
	state := store.CurrentMigrationState()
	return &state
}

// RetryDataMigration 重新执行一次数据迁移。
func (s *AppService) RetryDataMigration() *model.DataMigrationState {
	state := store.RetryDataMigration()
	return &state
}
