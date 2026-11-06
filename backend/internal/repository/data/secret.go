package data

import (
	"context"
	"fmt"
	"sort"

	"github.com/tomasgdl/infosentinel/backend/internal/crypto"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/secret"
)

// SetSecret 实现 repository.SecretRepository:明文经 AES-256-GCM 加密后落库。
func (d *Data) SetSecret(ctx context.Context, name, plaintext string) error {
	enc, err := crypto.Encrypt(plaintext)
	if err != nil {
		return err
	}
	err = d.client.Secret.Create().
		SetName(name).
		SetValue(enc).
		OnConflict().
		UpdateValue().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("保存敏感数据 %s: %w", name, err)
	}
	return nil
}

// GetSecret 实现 repository.SecretRepository:解密返回明文;不存在返回 ErrNotFound。
func (d *Data) GetSecret(ctx context.Context, name string) (string, error) {
	row, err := d.client.Secret.Query().Where(secret.Name(name)).First(ctx)
	if ent.IsNotFound(err) {
		return "", repository.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("读取敏感数据 %s: %w", name, err)
	}
	plain, err := crypto.Decrypt(row.Value)
	if err != nil {
		return "", fmt.Errorf("解密敏感数据 %s: %w", name, err)
	}
	return plain, nil
}

// DeleteSecret 实现 repository.SecretRepository。
func (d *Data) DeleteSecret(ctx context.Context, name string) error {
	n, err := d.client.Secret.Delete().Where(secret.Name(name)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("敏感数据 %s 不存在", name)
	}
	return nil
}

// ListSecrets 实现 repository.SecretRepository:只返回名称,不返回值。
func (d *Data) ListSecrets(ctx context.Context) ([]string, error) {
	rows, err := d.client.Secret.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询敏感数据: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out, nil
}

var _ repository.SecretRepository = (*Data)(nil)
