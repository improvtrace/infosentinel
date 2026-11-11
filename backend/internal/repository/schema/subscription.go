package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Subscription 订阅:{name(唯一标识), topics(JSON 数组), synthesis(合成提示词),
// cron(单个表达式), targets(JSON 数组), space(产物空间,单个)}。
// 订阅与主题多对多:一个订阅可关联多个主题,一个主题可被多个订阅引用。
type Subscription struct {
	ent.Schema
}

func (Subscription) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique().NotEmpty().MaxLen(64),
		field.JSON("topics", []string{}),
		field.String("synthesis").NotEmpty(),
		field.String("cron").NotEmpty(),
		field.JSON("targets", []string{}),
		field.String("space").Default(""),
	}
}

func (Subscription) Annotations() []schema.Annotation {
	return nil
}
