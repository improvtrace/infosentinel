package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Task 任务:{topic, targets(JSON 数组), schedules(JSON 数组, cron 表达式)}。
type Task struct {
	ent.Schema
}

func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique().NotEmpty().MaxLen(64),
		field.String("topic").NotEmpty(),
		field.JSON("targets", []string{}),
		field.JSON("schedules", []string{}),
	}
}

func (Task) Annotations() []schema.Annotation {
	return nil
}
