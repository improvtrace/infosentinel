package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Secret 敏感数据:AES-256-GCM 加密后存储(value 为密文),
// 名称约定:llm.api_key / smtp.password;明文永不落库。
type Secret struct {
	ent.Schema
}

func (Secret) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique().NotEmpty().MaxLen(128),
		field.Text("value").NotEmpty(),
	}
}

func (Secret) Annotations() []schema.Annotation {
	return nil
}
