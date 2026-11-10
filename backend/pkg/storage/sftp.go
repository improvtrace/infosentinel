package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strconv"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpStore 是 SFTP 驱动:每次操作建立短连接(连接 → 执行 → 关闭),
// 避免长时间闲置的会话被服务端断开后产生陈旧连接问题。
type sftpStore struct {
	cfg Config
	hkc ssh.HostKeyCallback
}

// newSFTP 创建 SFTP 驱动并校验参数。
func newSFTP(cfg Config) (*sftpStore, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("sftp.host 未配置")
	}
	hkc := ssh.InsecureIgnoreHostKey() //nolint:gosec // 未配置 host_key 时不校验指纹,在界面提示风险
	if cfg.HostKey != "" {
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(cfg.HostKey))
		if err != nil {
			return nil, fmt.Errorf("解析 sftp.host_key(authorized_keys 单行格式): %w", err)
		}
		hkc = ssh.FixedHostKey(pub)
	}
	return &sftpStore{cfg: cfg, hkc: hkc}, nil
}

// dial 建立一条 SFTP 连接,返回客户端与关闭函数。
func (s *sftpStore) dial(ctx context.Context) (*sftp.Client, func(), error) {
	port := s.cfg.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(port))
	timeout := timeoutOf(s.cfg.TimeoutSec)

	auth := make([]ssh.AuthMethod, 0, 2)
	if s.cfg.KeyPath != "" {
		key, err := os.ReadFile(s.cfg.KeyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("读取私钥 %s: %w", s.cfg.KeyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, nil, fmt.Errorf("解析私钥 %s: %w", s.cfg.KeyPath, err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if s.cfg.Password != "" {
		auth = append(auth, ssh.Password(s.cfg.Password))
	}
	if len(auth) == 0 {
		return nil, nil, fmt.Errorf("SFTP 认证未配置:请设置密码(storage.password)或私钥(sftp.key_path)")
	}

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("连接 SFTP %s: %w", addr, err)
	}
	cfg := &ssh.ClientConfig{
		User:            s.cfg.Username,
		Auth:            auth,
		HostKeyCallback: s.hkc,
		Timeout:         timeout,
	}
	sc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("SSH 握手 %s: %w", addr, err)
	}
	client, err := sftp.NewClient(ssh.NewClient(sc, chans, reqs))
	if err != nil {
		_ = sc.Close()
		return nil, nil, fmt.Errorf("创建 SFTP 会话: %w", err)
	}
	return client, func() { _ = client.Close() }, nil
}

// resolve 拼接并校验远端路径。
func (s *sftpStore) resolve(p string) (string, error) {
	c, err := CleanRel(p)
	if err != nil {
		return "", err
	}
	return joinRoot(s.cfg.Root, c), nil
}

// Save 实现 Store:写入文件(自动创建父目录)。
func (s *sftpStore) Save(ctx context.Context, p string, data []byte) error {
	full, err := s.resolve(p)
	if err != nil {
		return err
	}
	client, done, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err := client.MkdirAll(path.Dir(full)); err != nil {
		return fmt.Errorf("创建远端目录 %s: %w", path.Dir(full), err)
	}
	f, err := client.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("打开远端文件 %s: %w", full, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("写入远端文件 %s: %w", full, err)
	}
	return nil
}

// Read 实现 Store。
func (s *sftpStore) Read(ctx context.Context, p string) ([]byte, error) {
	full, err := s.resolve(p)
	if err != nil {
		return nil, err
	}
	client, done, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	f, err := client.OpenFile(full, os.O_RDONLY)
	if err != nil {
		return nil, wrapSFTP("读取远端文件 %s", p, err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("读取远端文件 %s: %w", p, err)
	}
	return raw, nil
}

// List 实现 Store:列出一级条目。
func (s *sftpStore) List(ctx context.Context, dir string) ([]Entry, error) {
	full, err := s.resolve(dir)
	if err != nil {
		return nil, err
	}
	client, done, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	if _, err := client.Stat(full); err != nil {
		return nil, wrapSFTP("列出远端目录 %s", dir, err)
	}
	infos, err := client.ReadDir(full)
	if err != nil {
		return nil, fmt.Errorf("列出远端目录 %s: %w", dir, err)
	}
	out := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, Entry{Name: fi.Name(), Size: fi.Size(), Dir: fi.IsDir(), ModTime: fi.ModTime()})
	}
	return out, nil
}

// Delete 实现 Store:删除文件。
func (s *sftpStore) Delete(ctx context.Context, p string) error {
	full, err := s.resolve(p)
	if err != nil {
		return err
	}
	client, done, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err := client.Remove(full); err != nil {
		return wrapSFTP("删除远端文件 %s", p, err)
	}
	return nil
}

// DeleteDir 实现 Store:递归删除目录(先清空内容,再删目录本身)。
func (s *sftpStore) DeleteDir(ctx context.Context, dir string) error {
	full, err := s.resolve(dir)
	if err != nil {
		return err
	}
	client, done, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err := sftpRemoveAll(client, full); err != nil {
		return fmt.Errorf("删除远端目录 %s: %w", dir, err)
	}
	return nil
}

// sftpRemoveAll 递归删除远端目录。
func sftpRemoveAll(client *sftp.Client, full string) error {
	infos, err := client.ReadDir(full)
	if err != nil {
		return err
	}
	for _, fi := range infos {
		child := full + "/" + fi.Name()
		if fi.IsDir() {
			if err := sftpRemoveAll(client, child); err != nil {
				return err
			}
		} else if err := client.Remove(child); err != nil {
			return err
		}
	}
	return client.RemoveDirectory(full)
}

// wrapSFTP 统一转换"不存在"错误并包装操作上下文。
func wrapSFTP(op, p string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, p)
	}
	return fmt.Errorf("%s: %w", op, err)
}

var _ Store = (*sftpStore)(nil)
