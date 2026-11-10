// Package schema 定义 InfoSentinel 配置存储的 ent 实体。
// 代码生成:hack/gen.sh(ent generate ./schema → ./ent)。
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionRun 订阅运行记录:每次调度/手动触发一行,随订阅执行持续增长。
// 表名 subscription_runs(ent 默认复数命名)。
type SubscriptionRun struct {
	ent.Schema
}

func (SubscriptionRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("topic").NotEmpty().MaxLen(64),
		field.String("trigger").NotEmpty().MaxLen(16),
		field.String("status").NotEmpty().MaxLen(16),
		field.Time("started_at"),
		field.Time("finished_at").Optional(),
		field.Int64("duration").Default(0),
		field.Text("error").Default(""),
		field.Text("subject").Default(""),
		field.Text("body").Default(""),
	}
}

// Indexes 运行记录是只追加的日志型数据,按订阅过滤、按时间倒序查询,建索引加速。
func (SubscriptionRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("topic", "started_at"),
		index.Fields("started_at"),
	}
}

func (SubscriptionRun) Annotations() []schema.Annotation {
	return nil
}
