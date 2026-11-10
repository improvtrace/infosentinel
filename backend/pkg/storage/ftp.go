package storage

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/jlaffaye/ftp"
)

// ftpStore 是 FTP/FTPS 驱动:每次操作建立短连接。
type ftpStore struct {
	cfg Config
}

// newFTP 创建 FTP 驱动并校验参数。
func newFTP(cfg Config) (*ftpStore, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("ftp.host 未配置")
	}
	return &ftpStore{cfg: cfg}, nil
}

// dial 建立一条 FTP 连接并登录,返回连接与关闭函数。
func (s *ftpStore) dial(ctx context.Context) (*ftp.ServerConn, func(), error) {
	port := s.cfg.Port
	if port == 0 {
		port = 21
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(port))
	timeout := timeoutOf(s.cfg.TimeoutSec)

	opts := []ftp.DialOption{ftp.DialWithContext(ctx), ftp.DialWithTimeout(timeout)}
	if s.cfg.TLS {
		opts = append(opts, ftp.DialWithExplicitTLS(&tls.Config{ServerName: s.cfg.Host}))
	}
	conn, err := ftp.Dial(addr, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("连接 FTP %s: %w", addr, err)
	}
	user := s.cfg.Username
	if user == "" {
		user = "anonymous"
	}
	pass := s.cfg.Password
	if pass == "" {
		pass = "anonymous"
	}
	if err := conn.Login(user, pass); err != nil {
		_ = conn.Quit()
		return nil, nil, fmt.Errorf("FTP 登录 %s: %w", addr, err)
	}
	return conn, func() { _ = conn.Quit() }, nil
}

// resolve 拼接并校验远端路径。
func (s *ftpStore) resolve(p string) (string, error) {
	c, err := CleanRel(p)
	if err != nil {
		return "", err
	}
	return joinRoot(s.cfg.Root, c), nil
}

// Save 实现 Store:写入文件(逐级创建缺失目录;FTP 无递归建目录命令)。
func (s *ftpStore) Save(ctx context.Context, p string, data []byte) error {
	full, err := s.resolve(p)
	if err != nil {
		return err
	}
	conn, done, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer done()
	// 逐级创建缺失目录(FTP 无递归建目录命令;已存在时忽略错误)
	parts := strings.Split(full, "/")
	cur := ""
	if strings.HasPrefix(full, "/") {
		cur = "/"
	}
	for _, seg := range parts[:max(len(parts)-1, 0)] {
		if seg == "" {
			continue
		}
		switch cur {
		case "/":
			cur += seg
		case "":
			cur = seg
		default:
			cur += "/" + seg
		}
		_ = conn.MakeDir(cur)
	}
	if err := conn.Stor(full, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("写入远端文件 %s: %w", full, err)
	}
	return nil
}

// Read 实现 Store。
func (s *ftpStore) Read(ctx context.Context, p string) ([]byte, error) {
	full, err := s.resolve(p)
	if err != nil {
		return nil, err
	}
	conn, done, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	resp, err := conn.Retr(full)
	if err != nil {
		return nil, wrapFTP("读取远端文件 %s", p, err)
	}
	defer func() { _ = resp.Close() }()
	raw, err := io.ReadAll(resp)
	if err != nil {
		return nil, fmt.Errorf("读取远端文件 %s: %w", p, err)
	}
	return raw, nil
}

// List 实现 Store:列出一级条目。
func (s *ftpStore) List(ctx context.Context, dir string) ([]Entry, error) {
	full, err := s.resolve(dir)
	if err != nil {
		return nil, err
	}
	conn, done, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	ents, err := conn.List(full)
	if err != nil {
		return nil, wrapFTP("列出远端目录 %s", dir, err)
	}
	out := make([]Entry, 0, len(ents))
	for _, e := range ents {
		out = append(out, Entry{
			Name:    e.Name,
			Size:    int64(e.Size),
			Dir:     e.Type == ftp.EntryTypeFolder,
			ModTime: e.Time,
		})
	}
	return out, nil
}

// Delete 实现 Store:删除文件。
func (s *ftpStore) Delete(ctx context.Context, p string) error {
	full, err := s.resolve(p)
	if err != nil {
		return err
	}
	conn, done, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err := conn.Delete(full); err != nil {
		return wrapFTP("删除远端文件 %s", p, err)
	}
	return nil
}

// DeleteDir 实现 Store:递归删除目录(库内置 RemoveDirRecur)。
func (s *ftpStore) DeleteDir(ctx context.Context, dir string) error {
	full, err := s.resolve(dir)
	if err != nil {
		return err
	}
	conn, done, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer done()
	// 先探测存在性(FTP 无 Stat,用 List)
	if _, err := conn.List(full); err != nil {
		return wrapFTP("列出远端目录 %s", dir, err)
	}
	if err := conn.RemoveDirRecur(full); err != nil {
		return fmt.Errorf("删除远端目录 %s: %w", dir, err)
	}
	return nil
}

// wrapFTP 统一转换"不存在"错误(FTP 协议 550 = File unavailable)并包装操作上下文。
func wrapFTP(op, p string, err error) error {
	var terr *textproto.Error
	if errors.As(err, &terr) && terr.Code == ftp.StatusFileUnavailable {
		return fmt.Errorf("%w: %s", ErrNotFound, p)
	}
	return fmt.Errorf("%s: %w", op, err)
}

var _ Store = (*ftpStore)(nil)
