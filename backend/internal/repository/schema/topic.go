// Package schema 定义 InfoSentinel 配置存储的 ent 实体。
// 代码生成:hack/gen.sh(ent generate ./schema → ./ent)。
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Topic 主题:name 为唯一标识,prompt 为 agent 指令。
type Topic struct {
	ent.Schema
}

func (Topic) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique().NotEmpty().MaxLen(64),
		field.Text("prompt").NotEmpty(),
	}
}

func (Topic) Annotations() []schema.Annotation {
	return nil
}
