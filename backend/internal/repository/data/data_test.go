package data

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// newTestData 创建临时 SQLite 数据层(New 会执行迁移,写入内置默认配置)。
func newTestData(t *testing.T) *Data {
	t.Helper()
	d, err := New(Options{DBPath: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// seedFixture 在内置数据之外直插测试用的业务行(tp1 主题 / m1 目标 / 订阅A)。
func seedFixture(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	if err := d.client.Topic.Create().SetName("tp1").SetPrompt("关注 AI").Exec(ctx); err != nil {
		t.Fatalf("插入测试主题: %v", err)
	}
	if err := d.client.Target.Create().SetName("m1").SetType("email").SetTo("a@b.c").SetSubjectPrefix("[IS]").Exec(ctx); err != nil {
		t.Fatalf("插入测试目标: %v", err)
	}
	if err := d.client.Subscription.Create().
		SetName("订阅A").SetTopics([]string{"tp1"}).SetSynthesis(repository.DefaultSynthesis).
		SetCron("0 8 * * *").SetTargets([]string{"m1"}).SetSpace(repository.DefaultSpaceName).
		Exec(ctx); err != nil {
		t.Fatalf("插入测试订阅: %v", err)
	}
}

// builtinTopicNames 是 init 迁移内置的主题名(与迁移 SQL 保持一致)。
var builtinTopicNames = []string{"world-situation", "ai-development", "security", "policy"}

// TestMigrateWritesBuiltinData 验证 init 迁移写入内置默认配置:
// LoadConfig(含 Finalize 完整校验)成功即证明主题/订阅/推送目标/全局设置齐备且合法。
func TestMigrateWritesBuiltinData(t *testing.T) {
	d := newTestData(t)

	cfg, err := d.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("加载内置默认配置: %v", err)
	}
	for _, name := range builtinTopicNames {
		if cfg.TopicByName(name) == nil {
			t.Errorf("内置主题 %s 缺失", name)
		}
	}
	sub := cfg.SubscriptionByName("情报日报")
	if sub == nil {
		t.Fatal("内置订阅 情报日报 缺失")
	}
	if !slices.Equal(sub.Topics, builtinTopicNames) {
		t.Errorf("内置订阅 topics = %v,期望 %v", sub.Topics, builtinTopicNames)
	}
	if len(sub.Targets) != 1 || sub.Targets[0] != "mailbox" || sub.Space != repository.DefaultSpaceName {
		t.Errorf("内置订阅 targets/space 不符: %+v", sub)
	}
	if cfg.TargetByName("mailbox") == nil {
		t.Error("内置推送目标 mailbox 缺失")
	}
	if cfg.LLM.BaseURL == "" || cfg.Mailer.Type != repository.MailerTypeInbox || len(cfg.Storage.Spaces) != 1 {
		t.Errorf("全局设置不符: llm=%+v mailer=%+v storage=%+v", cfg.LLM, cfg.Mailer, cfg.Storage)
	}
}

func TestCRUDAndReferentialIntegrity(t *testing.T) {
	d := newTestData(t)
	ctx := context.Background()
	seedFixture(t, d)

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
	if err := d.DeleteSubscription(ctx, "ghost"); err == nil {
		t.Fatal("删除不存在订阅应报错")
	}
	// 按名称 upsert 订阅:同名词外的字段全部更新,不影响内置订阅
	sub := repository.Subscription{
		Name: "订阅A", Topics: []string{"tp1"}, Synthesis: repository.DefaultSynthesis,
		Cron: "30 20 * * 1-5", Targets: []string{"m1"}, Space: "",
	}
	if err := d.UpsertSubscription(ctx, sub); err != nil {
		t.Fatalf("UpsertSubscription: %v", err)
	}
	got, err := d.LoadConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated := got.SubscriptionByName("订阅A")
	if updated == nil || updated.Cron != "30 20 * * 1-5" || updated.Space != "" {
		t.Errorf("按名称 upsert 未生效: %+v", updated)
	}
	if got.SubscriptionByName("情报日报") == nil {
		t.Error("内置订阅不应受测试写入影响")
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
