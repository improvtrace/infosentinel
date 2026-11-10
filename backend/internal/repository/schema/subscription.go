package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Subscription 订阅:{topic(唯一标识), cron(单个表达式), targets(JSON 数组)}。
// 一个主题最多一条订阅:主题即订阅的身份,也用于隔离去重状态与产物分桶。
type Subscription struct {
	ent.Schema
}

func (Subscription) Fields() []ent.Field {
	return []ent.Field{
		field.String("topic").Unique().NotEmpty().MaxLen(64),
		field.String("cron").NotEmpty(),
		field.JSON("targets", []string{}),
	}
}

func (Subscription) Annotations() []schema.Annotation {
	return nil
}
