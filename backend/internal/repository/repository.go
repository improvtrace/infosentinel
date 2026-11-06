// Package repository 定义配置存储的数据库操作接口(每表一个文件),
// 具体实现位于 repository/data(ent ORM)。业务层只依赖本包接口。
package repository

import "errors"

// ErrNotFound 表示查询的实体不存在。
var ErrNotFound = errors.New("not found")

// Repository 是全部存储操作的聚合接口(由 repository/data 提供 ent 实现)。
type Repository interface {
	TopicRepository
	TargetRepository
	TaskRepository
	SettingRepository
	SecretRepository
	ConfigRepository
}
