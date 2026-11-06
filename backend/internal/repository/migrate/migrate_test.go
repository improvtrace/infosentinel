package migrate

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ariga.io/atlas/sql/migrate"

	_ "modernc.org/sqlite"
)

// openTestDB 打开临时 SQLite 数据库(与 data.New 的 DSN 参数一致)。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_fk=1"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("打开测试库: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestRunFreshAndIdempotent 验证新库初始化与重复执行幂等。
func TestRunFreshAndIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	res, err := Run(ctx, db)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.From != "" {
		t.Errorf("新库 From 应为空,得到 %q", res.From)
	}
	if res.To == "" || len(res.Applied) != 1 || res.Applied[0] != res.To {
		t.Errorf("新库应只应用初始迁移,得到 %+v", res)
	}

	// 业务表全部创建
	for _, table := range []string{"topics", "targets", "tasks", "settings", "secrets"} {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Errorf("表 %s 未创建: %v", table, err)
		}
	}
	// 版本记录与迁移文件对应
	revs, err := (&revisions{db: db}).ReadRevisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 1 || revs[0].Version != res.To {
		t.Errorf("版本记录不符: %+v", revs)
	}

	// 重复执行:无待应用迁移
	res2, err := Run(ctx, db)
	if err != nil {
		t.Fatalf("重复 Run: %v", err)
	}
	if res2.From != res.To || res2.To != res.To || len(res2.Applied) != 0 {
		t.Errorf("重复执行结果不符: %+v", res2)
	}
}

// TestRunAdoptsLegacyDatabase 模拟旧版(ent 自动迁移时代)数据库:
// 业务表已存在但无版本记录 → 首次迁移以基线采纳,数据保留,后续启动幂等。
func TestRunAdoptsLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	latest := mustLatestVersion(t)
	if _, err := Run(ctx, db); err != nil {
		t.Fatalf("初始化: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO topics (name, prompt) VALUES ('legacy', 'old prompt')`); err != nil {
		t.Fatalf("写入旧数据: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE atlas_schema_revisions`); err != nil {
		t.Fatalf("删除版本表: %v", err)
	}

	// 升级:基线采纳,不重复建表
	res, err := Run(ctx, db)
	if err != nil {
		t.Fatalf("升级旧库: %v", err)
	}
	if len(res.Applied) != 0 || res.To != latest {
		t.Errorf("旧库应基线采纳而不重复执行迁移,得到 %+v(期望 To=%s)", res, latest)
	}
	var prompt string
	if err := db.QueryRowContext(ctx,
		`SELECT prompt FROM topics WHERE name = 'legacy'`).Scan(&prompt); err != nil {
		t.Fatalf("旧数据丢失: %v", err)
	}
	if prompt != "old prompt" {
		t.Errorf("旧数据被修改: %q", prompt)
	}
	revs, err := (&revisions{db: db}).ReadRevisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 1 || revs[0].Version != latest || revs[0].Type != migrate.RevisionTypeBaseline {
		t.Errorf("应记录一条基线版本(RevisionTypeBaseline): %+v", revs)
	}

	// 再次执行:幂等
	res2, err := Run(ctx, db)
	if err != nil {
		t.Fatalf("重开 Run: %v", err)
	}
	if len(res2.Applied) != 0 || res2.From != latest {
		t.Errorf("重开结果不符: %+v", res2)
	}
}

// TestRunRejectsNewerRevision 验证数据库版本高于迁移目录时拒绝执行。
func TestRunRejectsNewerRevision(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if _, err := Run(ctx, db); err != nil {
		t.Fatalf("初始化: %v", err)
	}
	// 伪造一条比迁移目录更新的版本记录(模拟数据库被更高版本程序迁移过)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO atlas_schema_revisions (version, description, type, applied, total,
		 executed_at, execution_time, hash)
		 VALUES ('99999999999999', 'future', 1, 1, 1, current_timestamp, 0, 'x')`); err != nil {
		t.Fatalf("写入未来版本: %v", err)
	}

	if _, err := Run(ctx, db); err == nil {
		t.Fatal("数据库版本高于迁移目录应报错")
	}
}

// TestMigrationDirIntegrity 验证内嵌迁移目录可通过 atlas 校验和检查。
func TestMigrationDirIntegrity(t *testing.T) {
	dir, err := migrationDir()
	if err != nil {
		t.Fatalf("migrationDir: %v", err)
	}
	files, err := dir.Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("迁移目录为空")
	}
	for _, f := range files {
		if f.Version() == "" || f.Desc() == "" {
			t.Errorf("迁移文件名不符合 <版本>_<描述>.sql: %q", f.Name())
		}
	}
	// 校验和应与 ent 生成的 atlas.sum 一致(Executor.Pending 内部同样校验)
	sum, err := dir.Checksum()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Sum() == "" {
		t.Fatal("校验和为空")
	}
}

// TestRevisionsRoundTrip 验证版本记录存储的读写删。
func TestRevisionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rrw := &revisions{db: db}
	if err := rrw.ensureTable(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := rrw.ReadRevision(ctx, "nope"); !errors.Is(err, migrate.ErrRevisionNotExist) {
		t.Errorf("不存在的版本应返回 ErrRevisionNotExist,得到 %v", err)
	}

	rev := revisionForTest("20261105000001", "test")
	if err := rrw.WriteRevision(ctx, rev); err != nil {
		t.Fatalf("WriteRevision: %v", err)
	}
	if err := rrw.WriteRevision(ctx, revisionForTest("20261105000002", "test2")); err != nil {
		t.Fatalf("WriteRevision: %v", err)
	}

	got, err := rrw.ReadRevision(ctx, "20261105000001")
	if err != nil {
		t.Fatalf("ReadRevision: %v", err)
	}
	if got.Version != rev.Version || got.Description != rev.Description ||
		got.Total != rev.Total || got.Hash != rev.Hash || got.ExecutedAt.IsZero() {
		t.Errorf("回读不符: %+v", got)
	}

	// 覆盖写(模拟部分应用进度)
	rev.Applied = 1
	rev.PartialHashes = []string{"h1:abc"}
	if err := rrw.WriteRevision(ctx, rev); err != nil {
		t.Fatalf("覆盖写: %v", err)
	}
	got, err = rrw.ReadRevision(ctx, "20261105000001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Applied != 1 || len(got.PartialHashes) != 1 {
		t.Errorf("覆盖写未生效: %+v", got)
	}

	revs, err := rrw.ReadRevisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 2 || revs[len(revs)-1].Version != "20261105000002" {
		t.Errorf("ReadRevisions 顺序不符: %d 条", len(revs))
	}

	if err := rrw.DeleteRevision(ctx, "20261105000001"); err != nil {
		t.Fatalf("DeleteRevision: %v", err)
	}
	if _, err := rrw.ReadRevision(ctx, "20261105000001"); !errors.Is(err, migrate.ErrRevisionNotExist) {
		t.Error("删除后应 ErrRevisionNotExist")
	}
}

// mustLatestVersion 返回迁移目录中的最新版本号。
func mustLatestVersion(t *testing.T) string {
	t.Helper()
	dir, err := migrationDir()
	if err != nil {
		t.Fatal(err)
	}
	files, err := dir.Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("迁移目录为空")
	}
	return files[len(files)-1].Version()
}

// revisionForTest 构造测试用版本记录。
func revisionForTest(version, desc string) *migrate.Revision {
	return &migrate.Revision{
		Version: version, Description: desc, Type: migrate.RevisionTypeExecute,
		Applied: 2, Total: 2,
		ExecutedAt: time.Now().UTC(), ExecutionTime: 1000,
		Hash: "h1:test",
	}
}
