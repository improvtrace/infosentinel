// Package migrate 数据库版本化迁移(ent/Atlas 标准方案)的运行时执行器。
//
// 迁移文件由 ent 生成,位于本目录 migrations/(schema 变更后执行
// `make migrate-diff NAME=xxx` 追加),内嵌进二进制;atlas.sum 保证目录完整性。
//
// 执行器为 atlas sql/migrate.Executor:
//   - 版本记录写入 atlas_schema_revisions,每个版本只执行一次;
//   - 逐语句执行并记录进度,中断后可从断点续跑;
//   - 目录校验和不一致时报错,防止篡改迁移文件。
//
// 兼容旧库:本项目早期用 ent 自动迁移(`Schema.Create`)建表,这类库没有
// revisions 表;首次迁移时检测到库非空(NotCleanError)则以最新版本为基线
// (baseline)登记,不再重复建表。
package migrate

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"ariga.io/atlas/sql/migrate"
	atlassqlite "ariga.io/atlas/sql/sqlite"
)

//go:embed all:migrations
var migrationsFS embed.FS

// Result 描述一次迁移执行结果。
type Result struct {
	From    string   // 执行前已应用的最新版本;空表示新库/无记录
	To      string   // 执行后的最新版本
	Applied []string // 本次执行的版本号
}

// migrationDir 把内嵌的迁移目录还原为 atlas Dir(含 atlas.sum 完整性校验)。
func migrationDir() (*migrate.MemDir, error) {
	const root = "migrations"
	dir := &migrate.MemDir{}
	err := fs.WalkDir(migrationsFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := migrationsFS.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return dir.WriteFile(name, b)
	})
	if err != nil {
		return nil, fmt.Errorf("读取内嵌迁移目录: %w", err)
	}
	return dir, nil
}

// Run 将数据库迁移到内嵌迁移目录的最新版本。幂等:重复调用不重复执行。
func Run(ctx context.Context, db *sql.DB) (Result, error) {
	var res Result
	dir, err := migrationDir()
	if err != nil {
		return res, err
	}
	drv, err := atlassqlite.Open(db)
	if err != nil {
		return res, fmt.Errorf("创建迁移驱动: %w", err)
	}
	rrw := &revisions{db: db}
	if err := rrw.ensureTable(ctx); err != nil {
		return res, err
	}

	if res.From, err = rrw.lastVersion(ctx); err != nil {
		return res, err
	}

	files, err := dir.Files()
	if err != nil {
		return res, fmt.Errorf("读取迁移文件: %w", err)
	}
	if len(files) == 0 {
		res.To = res.From
		return res, nil
	}
	latest := files[len(files)-1].Version()

	ex, err := migrate.NewExecutor(drv, dir, rrw)
	if err != nil {
		return res, fmt.Errorf("创建迁移执行器: %w", err)
	}
	pending, err := ex.Pending(ctx)
	switch {
	case err == nil:
	case errors.Is(err, migrate.ErrNoPendingFiles):
		res.To = latestApplied(res.From, latest)
		return res, nil
	default:
		// 库非空且无版本记录(旧版自动迁移时代的数据库)→ 以最新版本为基线采纳,
		// 不重复建表;之后的增量迁移正常执行。
		var nce *migrate.NotCleanError
		if !errors.As(err, &nce) {
			return res, fmt.Errorf("计算待执行迁移: %w", err)
		}
		ex, err = migrate.NewExecutor(drv, dir, rrw, migrate.WithBaselineVersion(latest))
		if err != nil {
			return res, fmt.Errorf("创建基线迁移执行器: %w", err)
		}
		pending, err = ex.Pending(ctx)
		if err != nil && !errors.Is(err, migrate.ErrNoPendingFiles) {
			return res, fmt.Errorf("计算基线后待执行迁移: %w", err)
		}
		res.To = latest
	}

	for _, f := range pending {
		if err := ex.Execute(ctx, f); err != nil {
			return res, fmt.Errorf("执行迁移 %s(%s): %w", f.Version(), f.Desc(), err)
		}
		res.Applied = append(res.Applied, f.Version())
	}
	if len(res.Applied) > 0 || res.To == "" {
		res.To = res.Applied[len(res.Applied)-1]
		if res.To == "" {
			res.To = latest
		}
	}
	return res, nil
}

// latestApplied 返回执行后的版本:优先最后记录,否则取目录最新版本。
func latestApplied(from, latest string) string {
	if from != "" {
		return from
	}
	return latest
}

// revisions 是 atlas migrate.RevisionReadWriter 的 SQLite 实现,
// 表结构与 Atlas CLI 兼容(atlas_schema_revisions)。
type revisions struct {
	db *sql.DB
}

const revisionsTable = "atlas_schema_revisions"

// ensureTable 创建版本记录表。
func (r *revisions) ensureTable(ctx context.Context) error {
	const ddl = `CREATE TABLE IF NOT EXISTS atlas_schema_revisions (
    version CHARACTER VARYING(191) PRIMARY KEY,
    description CHARACTER VARYING(191) NOT NULL,
    type BIGINT NOT NULL,
    applied BIGINT NOT NULL,
    total BIGINT NOT NULL,
    executed_at TIMESTAMP NOT NULL,
    execution_time BIGINT NOT NULL,
    error CHARACTER VARYING(512) NULL,
    error_stmt TEXT NULL,
    hash CHARACTER VARYING(255) NOT NULL,
    partial_hashes TEXT NULL,
    operator_version CHARACTER VARYING(63) NULL
);`
	if _, err := r.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("创建 %s: %w", revisionsTable, err)
	}
	return nil
}

// Ident 实现 migrate.RevisionReadWriter。
func (r *revisions) Ident() *migrate.TableIdent {
	return &migrate.TableIdent{Name: revisionsTable}
}

// ReadRevisions 实现 migrate.RevisionReadWriter:按写入(执行)顺序返回全部版本。
func (r *revisions) ReadRevisions(ctx context.Context) ([]*migrate.Revision, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT version, description, type, applied, total,
		executed_at, execution_time, error, error_stmt, hash, partial_hashes, operator_version
		FROM `+revisionsTable+` ORDER BY rowid ASC`)
	if err != nil {
		return nil, fmt.Errorf("读取迁移版本: %w", err)
	}
	defer rows.Close()

	var out []*migrate.Revision
	for rows.Next() {
		rev, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取迁移版本: %w", err)
	}
	return out, nil
}

// ReadRevision 实现 migrate.RevisionReadWriter。
func (r *revisions) ReadRevision(ctx context.Context, v string) (*migrate.Revision, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT version, description, type, applied, total,
		executed_at, execution_time, error, error_stmt, hash, partial_hashes, operator_version
		FROM `+revisionsTable+` WHERE version = ?`, v)
	if err != nil {
		return nil, fmt.Errorf("读取迁移版本 %s: %w", v, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, migrate.ErrRevisionNotExist
	}
	return scanRevision(rows)
}

// WriteRevision 实现 migrate.RevisionReadWriter(整行覆盖)。
func (r *revisions) WriteRevision(ctx context.Context, rev *migrate.Revision) error {
	partial, err := json.Marshal(rev.PartialHashes)
	if err != nil {
		return fmt.Errorf("序列化 partial_hashes: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO `+revisionsTable+`
		(version, description, type, applied, total, executed_at, execution_time, error, error_stmt, hash, partial_hashes, operator_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(version) DO UPDATE SET
			description=excluded.description, type=excluded.type, applied=excluded.applied,
			total=excluded.total, executed_at=excluded.executed_at, execution_time=excluded.execution_time,
			error=excluded.error, error_stmt=excluded.error_stmt, hash=excluded.hash,
			partial_hashes=excluded.partial_hashes, operator_version=excluded.operator_version`,
		rev.Version, rev.Description, rev.Type, rev.Applied, rev.Total,
		rev.ExecutedAt, rev.ExecutionTime, rev.Error, rev.ErrorStmt, rev.Hash,
		string(partial), rev.OperatorVersion,
	)
	if err != nil {
		return fmt.Errorf("写入迁移版本 %s: %w", rev.Version, err)
	}
	return nil
}

// DeleteRevision 实现 migrate.RevisionReadWriter。
func (r *revisions) DeleteRevision(ctx context.Context, v string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM `+revisionsTable+` WHERE version = ?`, v); err != nil {
		return fmt.Errorf("删除迁移版本 %s: %w", v, err)
	}
	return nil
}

// lastVersion 返回已应用的最新版本(无记录为空)。
func (r *revisions) lastVersion(ctx context.Context) (string, error) {
	revs, err := r.ReadRevisions(ctx)
	if err != nil {
		return "", err
	}
	if len(revs) == 0 {
		return "", nil
	}
	return revs[len(revs)-1].Version, nil
}

// scanner 抽象 sql.Rows/Row 的 Scan,供两个读取方法复用。
type scanner interface{ Scan(dest ...any) error }

// scanRevision 把一行记录转换为 migrate.Revision。
func scanRevision(s scanner) (*migrate.Revision, error) {
	var (
		rev        migrate.Revision
		partialRaw sql.NullString
	)
	err := s.Scan(&rev.Version, &rev.Description, &rev.Type, &rev.Applied, &rev.Total,
		&rev.ExecutedAt, &rev.ExecutionTime, &rev.Error, &rev.ErrorStmt, &rev.Hash,
		&partialRaw, &rev.OperatorVersion)
	if err != nil {
		return nil, fmt.Errorf("解析迁移版本: %w", err)
	}
	if partialRaw.Valid && partialRaw.String != "" {
		if err := json.Unmarshal([]byte(partialRaw.String), &rev.PartialHashes); err != nil {
			return nil, fmt.Errorf("解析 partial_hashes: %w", err)
		}
	}
	return &rev, nil
}
