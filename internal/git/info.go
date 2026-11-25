package git

import (
	"net/url"
	"regexp"
	"strings"
)

// RemoteAddress 包含远程名称、展示用地址以及 Fetch 或 Push 用途。
type RemoteAddress struct {
	Name, URL, Direction string
}

// ConfigEntry 记录一条配置及其来源、作用域和展示值。
type ConfigEntry struct {
	Scope, Origin, Key, Value string
}

// RepositoryInfo 是工作区、当前引用、远程及配置的只读快照。
type RepositoryInfo struct {
	GitDir    string
	Head      string
	Branch    BranchInfo
	Remotes   []RemoteAddress
	RemoteErr error
	Config    []ConfigEntry
	ConfigErr error
}

// Remotes 读取 Git 解析后的 Fetch 和 Push 地址，返回已隐藏凭据的地址列表及错误。
func (c *Client) Remotes() ([]RemoteAddress, error) {
	raw, err := c.Run("remote", "-v")
	if err != nil {
		return nil, err
	}
	var remotes []RemoteAddress
	for line := range strings.SplitSeq(raw, "\n") {
		name, details, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		// 部分克隆会在 (fetch) 后附加过滤器说明。
		const fetchSuffix = " (fetch)"
		if strings.HasSuffix(details, "]") {
			if i := strings.LastIndex(details, fetchSuffix+" ["); i >= 0 {
				details = details[:i+len(fetchSuffix)]
			}
		}
		if i := strings.LastIndex(details, " ("); i >= 0 {
			direction := strings.TrimSuffix(details[i+2:], ")")
			if direction == "fetch" || direction == "push" {
				remotes = append(remotes, RemoteAddress{name, redactDisplayURLs(details[:i]), direction})
			}
		}
	}
	return remotes, nil
}

// RepositoryInfo 读取当前工作区的详细 Git 信息。无输入参数，返回仓库快照。
func (c *Client) RepositoryInfo() RepositoryInfo {
	gitDir, _ := c.Run("rev-parse", "--absolute-git-dir")
	head, _ := c.Run("rev-parse", "--verify", "HEAD")
	branch, _ := c.GetBranchInfo()
	remotes, remoteErr := c.Remotes()
	entries, configErr := c.readConfig()
	return RepositoryInfo{
		GitDir: strings.TrimSpace(gitDir), Head: strings.TrimSpace(head), Branch: branch,
		Remotes: remotes, RemoteErr: remoteErr, Config: entries, ConfigErr: configErr,
	}
}

// readConfig 读取当前工作区可见的配置，保留重复项和来源，返回脱敏后的配置及错误。
func (c *Client) readConfig() ([]ConfigEntry, error) {
	args := []string{"config", "--null", "--show-scope", "--show-origin", "--list"}
	raw, err := c.run("", append([]string{"--no-pager"}, args...), args)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(raw, "\x00")
	var entries []ConfigEntry
	for i := 0; i+2 < len(fields); i += 3 {
		key, value, hasValue := strings.Cut(fields[i+2], "\n")
		if !hasValue {
			value = "true"
		}
		section, _, _ := strings.Cut(key, ".")
		name := key[strings.LastIndex(key, ".")+1:]
		switch {
		case value == "":
		case section == "credential",
			name == "extraheader", name == "cookie", name == "pass", name == "password",
			name == "smtppass", name == "token", name == "accesstoken", name == "secret":
			value = "••••••"
		default:
			value = redactDisplayURLs(value)
		}
		entries = append(entries, ConfigEntry{fields[i], fields[i+1], redactDisplayURLs(key), value})
	}
	return entries, nil
}

var displayURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s]+`)

// redactDisplayURLs 隐藏 text 内 URL 的 HTTP(S) 用户信息、密码、查询参数和片段，并返回展示文本。
func redactDisplayURLs(text string) string {
	return displayURL.ReplaceAllStringFunc(text, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return "[地址已隐藏]"
		}
		if u.User != nil {
			_, hasPassword := u.User.Password()
			if hasPassword || strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https") {
				u.User = nil
			}
		}
		u.RawQuery, u.Fragment, u.ForceQuery = "", "", false
		return u.String()
	})
}
