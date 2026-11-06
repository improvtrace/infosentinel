// Package secret 解析运行时敏感数据:环境变量优先,其次数据库密文(AES-GCM)。
//
// 约定名称:
//   - llm.api_key    模型 API Key(对应 llm.api_key_env 指向的环境变量)
//   - smtp.password  SMTP 授权码(对应 mailer.smtp.password_env 指向的环境变量)
//
// 环境变量优先便于容器/CI 注入;Web 界面写入的值加密存储于数据库。
package secret

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// 敏感数据名称。
const (
	NameLLMAPIKey    = "llm.api_key"
	NameSMTPPassword = "smtp.password"
)

// Resolver 解析敏感数据。
type Resolver struct {
	repo repository.SecretRepository
}

// New 创建解析器;repo 可为 nil(此时只读环境变量)。
func New(repo repository.SecretRepository) *Resolver {
	return &Resolver{repo: repo}
}

// LLMAPIKey 返回模型 API Key:环境变量 → 数据库密文。
func (r *Resolver) LLMAPIKey(ctx context.Context, cfg *repository.Config) (string, error) {
	return r.resolve(ctx, cfg.LLM.APIKeyEnv, NameLLMAPIKey, "模型 API Key")
}

// SMTPPassword 返回 SMTP 密码/授权码:环境变量 → 数据库密文。
func (r *Resolver) SMTPPassword(ctx context.Context, cfg *repository.Config) (string, error) {
	return r.resolve(ctx, cfg.Mailer.Smtp.PasswordEnv, NameSMTPPassword, "SMTP 密码/授权码")
}

// resolve 按 环境变量 → 数据库 的顺序解析;均缺失则返回明确错误。
func (r *Resolver) resolve(ctx context.Context, envName, secretName, label string) (string, error) {
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			return v, nil
		}
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
	if envName != "" {
		return "", fmt.Errorf("%s未配置:请设置环境变量 %s,或在 Web 界面“全局设置”中填写(加密存储)", label, envName)
	}
	return "", fmt.Errorf("%s未配置", label)
}

// Has 判断某敏感数据是否已配置(不返回值,供界面展示)。
func (r *Resolver) Has(ctx context.Context, name string) bool {
	if r.repo == nil {
		return false
	}
	_, err := r.repo.GetSecret(ctx, name)
	return err == nil
}
