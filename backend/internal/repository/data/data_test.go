package data

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// newTestData 创建临时 SQLite 数据层。
func newTestData(t *testing.T) *Data {
	t.Helper()
	d, err := New(Options{DBPath: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func seedConfig() *repository.Config {
	cfg := &repository.Config{
		Sources: repository.Sources{Search: repository.SearchConfig{Enabled: true}},
		Topics:  []repository.Topic{{Name: "tp1", Prompt: "关注 AI"}},
		Targets: []repository.Target{{Name: "m1", Type: "email", To: "a@b.c", SubjectPrefix: "[InfoSentinel]"}},
		Tasks: []repository.Task{{
			Name: "j1", Topic: "tp1", Targets: []string{"m1"}, Schedules: []string{"0 8 * * *", "30 20 * * *"},
		}},
		LLM:    repository.LLMConfig{BaseURL: "https://api.deepseek.com", Model: "deepseek-chat", APIKeyEnv: "DEEPSEEK_API_KEY", Temperature: 0.3, TimeoutSec: 300, MaxStep: 25},
		Mailer: repository.MailerConfig{Type: "dryrun", OutputDir: "output"},
	}
	if err := cfg.Finalize(); err != nil {
		panic(err)
	}
	return cfg
}

func TestMigrateAndSeedRoundTrip(t *testing.T) {
	d := newTestData(t)
	ctx := context.Background()

	// New 对空库自动写入 migrate 内置默认配置
	if !d.Seeded() {
		t.Fatal("空库应自动写入内置默认配置")
	}
	if empty, err := d.IsEmpty(ctx); err != nil || empty {
		t.Fatalf("内置种子导入后不应为空: empty=%v err=%v", empty, err)
	}
	builtin, err := d.LoadConfig(ctx)
	if err != nil {
		t.Fatalf("加载内置种子: %v", err)
	}
	if len(builtin.Topics) != 1 || builtin.Topics[0].Name != "ai-agent" ||
		builtin.Mailer.OutputDir != ".tests/output" || builtin.Sources.Collect.StateDir != ".tests/data/state" {
		t.Errorf("内置种子内容不符: %+v", builtin)
	}

	// 整体替换种子与回读
	seed := seedConfig()
	if err := d.SeedFromConfig(ctx, seed); err != nil {
		t.Fatalf("SeedFromConfig: %v", err)
	}
	if empty, _ := d.IsEmpty(ctx); empty {
		t.Fatal("种子导入后不应为空")
	}
	// 重复导入(整体替换)应幂等
	if err := d.SeedFromConfig(ctx, seed); err != nil {
		t.Fatalf("重复导入: %v", err)
	}

	got, err := d.LoadConfig(ctx)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.LLM.Model != "deepseek-chat" || got.Mailer.OutputDir != "output" {
		t.Errorf("settings 回读不符: %+v", got)
	}
	if len(got.Topics) != 1 || got.Topics[0].Prompt != "关注 AI" {
		t.Errorf("topics 回读不符: %+v", got.Topics)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].Schedules[1] != "30 20 * * *" {
		t.Errorf("tasks 回读不符: %+v", got.Tasks)
	}
	if !got.Sources.Search.Enabled {
		t.Errorf("sources 回读不符: %+v", got.Sources)
	}
}

func TestCRUDAndReferentialIntegrity(t *testing.T) {
	d := newTestData(t)
	ctx := context.Background()
	if err := d.SeedFromConfig(ctx, seedConfig()); err != nil {
		t.Fatal(err)
	}

	// 更新主题
	if err := d.UpsertTopic(ctx, repository.Topic{Name: "tp1", Prompt: "改过的 prompt"}); err != nil {
		t.Fatal(err)
	}
	if tp, _ := d.GetTopic(ctx, "tp1"); tp.Prompt != "改过的 prompt" {
		t.Errorf("upsert 未生效: %+v", tp)
	}
	// 删除被引用主题 → 报错
	if err := d.DeleteTopic(ctx, "tp1"); err == nil {
		t.Fatal("删除被引用主题应报错")
	}
	// 未引用主题可删
	_ = d.UpsertTopic(ctx, repository.Topic{Name: "tp2", Prompt: "p2"})
	if err := d.DeleteTopic(ctx, "tp2"); err != nil {
		t.Fatalf("删除未引用主题: %v", err)
	}
	if _, err := d.GetTopic(ctx, "tp2"); err != repository.ErrNotFound {
		t.Errorf("删除后应 ErrNotFound,得到 %v", err)
	}
	// 删除被引用目标 → 报错
	if err := d.DeleteTarget(ctx, "m1"); err == nil {
		t.Fatal("删除被引用目标应报错")
	}
	// 引用计数
	if n, _ := d.UsedByTopic(ctx, "tp1"); n != 1 {
		t.Errorf("UsedByTopic = %d", n)
	}
	if n, _ := d.UsedByTarget(ctx, "m1"); n != 1 {
		t.Errorf("UsedByTarget = %d", n)
	}
	// 删除不存在
	if err := d.DeleteTask(ctx, "ghost"); err == nil {
		t.Fatal("删除不存在任务应报错")
	}
}

func TestSecretsEncrypted(t *testing.T) {
	t.Setenv("INFOSENTINEL_ENCRYPT_KEY", "test-master-key")
	d := newTestData(t)
	ctx := context.Background()

	if err := d.SetSecret(ctx, "llm.api_key", "sk-secret-value"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	// 落库应为密文
	row, err := d.client.Secret.Query().First(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if row.Value == "sk-secret-value" || row.Value[:3] != "v1:" {
		t.Errorf("敏感数据应加密存储,得到 %q", row.Value)
	}
	// 解密回读
	got, err := d.GetSecret(ctx, "llm.api_key")
	if err != nil || got != "sk-secret-value" {
		t.Errorf("GetSecret = %q err=%v", got, err)
	}
	// 列表只回名称
	names, _ := d.ListSecrets(ctx)
	if len(names) != 1 || names[0] != "llm.api_key" {
		t.Errorf("ListSecrets = %v", names)
	}
	// 删除
	if err := d.DeleteSecret(ctx, "llm.api_key"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetSecret(ctx, "llm.api_key"); err != repository.ErrNotFound {
		t.Errorf("删除后应 ErrNotFound")
	}
}

func TestSecretsWithoutKeyFails(t *testing.T) {
	d := newTestData(t)
	if err := d.SetSecret(context.Background(), "llm.api_key", "v"); err == nil {
		t.Fatal("未配置主密钥时加密应失败")
	}
}
