// Package data 是 repository 接口的 ent ORM 实现:
// data.go 负责客户端初始化 / 迁移 / 聚合;其余文件每表一个实现。
package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动(无 CGO),注册名 "sqlite"

	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
	dbmigrate "github.com/tomasgdl/infosentinel/backend/internal/repository/migrate"
)

// MigrationResult 是数据库迁移结果(repository/migrate.Result 的别名)。
type MigrationResult = dbmigrate.Result

// Data 聚合 ent 客户端,是各表实现的宿主。
type Data struct {
	client          *ent.Client
	db              *sql.DB
	migrationResult MigrationResult
	seeded          bool
}

// Options 是数据层初始化参数。
type Options struct {
	// DBPath 是 SQLite 文件路径。
	DBPath string
}

// New 打开数据库、执行版本化迁移(ent/Atlas 标准流程,幂等)并返回 Data。
// 空库时写入 migrate 内置默认配置(种子)。
func New(opts Options) (*Data, error) {
	if dir := filepath.Dir(opts.DBPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录: %w", err)
		}
	}
	dsn := "file:" + opts.DBPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_fk=1"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s: %w", opts.DBPath, err)
	}
	// SQLite 单写者,收紧并发
	db.SetMaxOpenConns(1)

	// 版本化迁移:执行 repository/migrate 内嵌迁移目录
	res, err := dbmigrate.Run(context.Background(), db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	d := &Data{client: client, db: db, migrationResult: res}
	if err := d.seedBuiltins(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return d, nil
}

// MigrationResult 返回 New 初始化时执行的迁移结果。
func (d *Data) MigrationResult() MigrationResult { return d.migrationResult }

// Seeded 报告 New 初始化时是否写入了内置默认配置(空库种子)。
func (d *Data) Seeded() bool { return d.seeded }

// Close 关闭数据库。
func (d *Data) Close() error { return d.client.Close() }

// seedBuiltins 空库时写入 migrate 内置默认配置(migrate 子命令与首次启动均生效)。
func (d *Data) seedBuiltins(ctx context.Context) error {
	empty, err := d.IsEmpty(ctx)
	if err != nil {
		return fmt.Errorf("检查数据库是否为空: %w", err)
	}
	if !empty {
		return nil
	}
	seed, err := dbmigrate.Builtins()
	if err != nil {
		return err
	}
	if err := d.SeedFromConfig(ctx, seed); err != nil {
		return fmt.Errorf("写入内置默认配置: %w", err)
	}
	d.seeded = true
	return nil
}
