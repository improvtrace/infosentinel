package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Target 推送目标:name 为唯一标识,type 当前为 email。
type Target struct {
	ent.Schema
}

func (Target) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique().NotEmpty().MaxLen(64),
		field.String("type").Default("email").NotEmpty(),
		field.String("to").NotEmpty(),
		field.String("subject_prefix").Default(""),
	}
}

func (Target) Annotations() []schema.Annotation {
	return nil
}
