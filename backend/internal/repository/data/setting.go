package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/setting"
)

// GetSetting 实现 repository.SettingRepository:读取 key 的 JSON 段反序列化到 out。
func (d *Data) GetSetting(ctx context.Context, key string, out any) error {
	row, err := d.client.Setting.Query().Where(setting.Key(key)).First(ctx)
	if ent.IsNotFound(err) {
		return repository.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("读取设置 %s: %w", key, err)
	}
	if err := json.Unmarshal([]byte(row.Value), out); err != nil {
		return fmt.Errorf("解析设置 %s: %w", key, err)
	}
	return nil
}

// PutSetting 实现 repository.SettingRepository:序列化 v 并 upsert。
func (d *Data) PutSetting(ctx context.Context, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("序列化设置 %s: %w", key, err)
	}
	err = d.client.Setting.Create().
		SetKey(key).
		SetValue(string(raw)).
		OnConflict().
		UpdateValue().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("保存设置 %s: %w", key, err)
	}
	return nil
}

var _ repository.SettingRepository = (*Data)(nil)
