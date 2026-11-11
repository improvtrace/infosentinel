package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Setting 全局设置段:key ∈ {llm, mailer, server, storage},value 为该段 JSON。
type Setting struct {
	ent.Schema
}

func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique().NotEmpty(),
		field.Text("value").NotEmpty(),
	}
}

func (Setting) Annotations() []schema.Annotation {
	return nil
}
