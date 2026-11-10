package config

import (
	"context"
	"errors"
	"fmt"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// 敏感数据名称。
const (
	NameLLMAPIKey       = "llm.api_key"
	NameSMTPPassword    = "smtp.password"
	NameStoragePassword = "storage.password"
)

// Resolver 解析运行时敏感数据:仅从数据库密文读取(AES-GCM)。
//
// 约定名称:
//   - llm.api_key       模型 API Key
//   - smtp.password     SMTP 授权码
//   - storage.password  SFTP/FTP 登录密码
//
// 业务配置不读环境变量,Web 界面“全局设置”是唯一入口;
// 写入的值加密存储于数据库(主密钥为引导层的 INFOSENTINEL_ENCRYPT_KEY)。
type Resolver struct {
	repo repository.SecretRepository
}

// NewResolver 创建解析器;repo 可为 nil(此时所有敏感数据视为未配置)。
func NewResolver(repo repository.SecretRepository) *Resolver {
	return &Resolver{repo: repo}
}

// LLMAPIKey 返回模型 API Key(数据库密文)。
func (r *Resolver) LLMAPIKey(ctx context.Context) (string, error) {
	return r.ResolveWith(ctx, NameLLMAPIKey, "模型 API Key", "", true)
}

// SMTPPassword 返回 SMTP 密码/授权码(数据库密文)。
func (r *Resolver) SMTPPassword(ctx context.Context) (string, error) {
	return r.ResolveWith(ctx, NameSMTPPassword, "SMTP 密码/授权码", "", true)
}

// StoragePassword 返回 SFTP/FTP 登录密码:override → 数据库密文。
// 均缺失时返回空串(本地驱动/私钥登录无需密码,密码缺失错误由驱动在使用时报出)。
func (r *Resolver) StoragePassword(ctx context.Context, override string) (string, error) {
	return r.ResolveWith(ctx, NameStoragePassword, "SFTP/FTP 密码", override, false)
}

// ResolveWith 按 override → 数据库密文 的顺序解析敏感数据。
// override 非空时直接使用(供"测试"端点传入表单值,不落库);
// 均缺失时 required 为 true 返回明确错误,为 false 返回空串。
func (r *Resolver) ResolveWith(ctx context.Context, secretName, label, override string, required bool) (string, error) {
	if override != "" {
		return override, nil
	}
	if r.repo != nil {
		v, err := r.repo.GetSecret(ctx, secretName)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return "", err
		}
	}
	if required {
		return "", fmt.Errorf("%s未配置:请在 Web 界面“配置 → 全局设置”中填写并加密保存", label)
	}
	return "", nil
}

// Has 判断某敏感数据是否已配置(不返回值,供界面展示)。
func (r *Resolver) Has(ctx context.Context, name string) bool {
	if r.repo == nil {
		return false
	}
	_, err := r.repo.GetSecret(ctx, name)
	return err == nil
}
