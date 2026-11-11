// migrate-diff 生成 ent 版本化迁移文件(Atlas 目录格式)。
//
// 流程:用一次性内存 SQLite 重放现有迁移目录 → 与 ent schema 求差 →
// 写入 <UTC时间戳>_<name>.sql 并更新 atlas.sum。不触碰真实数据库。
//
// 手工调整生成的迁移 SQL(如保数据变换)后,校验和会失配,此时执行
// `go run ./backend/cmd/migrate-diff -hash` 仅按当前目录内容重算 atlas.sum。
//
// 用法(一般通过 make migrate-diff NAME=xxx 调用):
//
//	go run ./backend/cmd/migrate-diff -name add_tags
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"ariga.io/atlas/sql/migrate"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"

	entmigrate "github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/migrate"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动,注册名 "sqlite"
)

func main() {
	var name string
	var rehash bool
	flag.StringVar(&name, "name", "changes", "迁移描述(写入迁移文件名,如 add_tags)")
	flag.BoolVar(&rehash, "hash", false, "不生成迁移,仅按当前迁移目录内容重算 atlas.sum(手工调整 SQL 后使用)")
	flag.Parse()

	if err := run(name, rehash); err != nil {
		log.Fatalf("migrate-diff: %v", err)
	}
}

// moduleRoot 从 CWD 逐级向上查找 go.mod,定位模块根目录。
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("未找到 go.mod(请在模块内执行)")
		}
		dir = parent
	}
}

func run(name string, rehash bool) error {
	if !rehash && name == "" {
		return fmt.Errorf("迁移名不能为空(-name)")
	}
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	// 迁移目录固定位于 backend/internal/repository/migrate/migrations
	dirPath := filepath.Join(root, "backend", "internal", "repository", "migrate", "migrations")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("创建迁移目录 %s: %w", dirPath, err)
	}
	dir, err := migrate.NewLocalDir(dirPath)
	if err != nil {
		return fmt.Errorf("打开迁移目录 %s: %w", dirPath, err)
	}

	// -hash:按当前目录内容重算 atlas.sum(手工调整迁移 SQL 后使用)。
	if rehash {
		sum, err := dir.Checksum()
		if err != nil {
			return fmt.Errorf("计算迁移目录校验和: %w", err)
		}
		if err := migrate.WriteSumFile(dir, sum); err != nil {
			return fmt.Errorf("写入 atlas.sum: %w", err)
		}
		fmt.Printf("==> atlas.sum 已按 %s 重算\n", dirPath)
		return nil
	}

	// 一次性内存库:ModeReplay 在其上重放迁移目录得到当前状态,再与 ent schema 求差。
	// _fk=1 为 atlas sqlite 驱动强制要求(校验连接串)。
	db, err := sql.Open("sqlite", "file:ent_migrate_diff?mode=memory&cache=shared&_fk=1")
	if err != nil {
		return fmt.Errorf("打开内存数据库: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	sm := entmigrate.NewSchema(entsql.OpenDB(dialect.SQLite, db))
	if err := sm.NamedDiff(context.Background(), name,
		schema.WithDir(dir),
		schema.WithMigrationMode(schema.ModeReplay),
		// atlas 单文件格式(<版本>_<描述>.sql),运行时执行器按文件序执行
		schema.WithFormatter(migrate.DefaultFormatter),
	); err != nil {
		return fmt.Errorf("生成迁移文件: %w", err)
	}
	fmt.Printf("==> 迁移文件已写入 %s(name=%s)\n", dirPath, name)
	return nil
}
