package git

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// BranchInfo 描述仓库当前分支、本地所有分支以及与远程上游分支的领先/落后提交数。
type BranchInfo struct {
	Current  string
	Branches []string
	Upstream string
	Ahead    int
	Behind   int
}

var branchNameRegex = regexp.MustCompile(`^[A-Za-z0-9._/\-]+$`)

// GetBranchInfo 查询当前仓库所在的分支名、所有本地分支列表以及与上游分支的 Ahead/Behind 状态。
// 无输入参数，返回 BranchInfo 和可能的错误。
func (c *Client) GetBranchInfo() (BranchInfo, error) {
	var info BranchInfo

	cur, err := c.Run("symbolic-ref", "--short", "HEAD")
	if err == nil {
		info.Current = strings.TrimSpace(cur)
	} else {
		rev, revErr := c.Run("rev-parse", "--short", "HEAD")
		if revErr == nil {
			info.Current = "HEAD (" + strings.TrimSpace(rev) + ")"
		} else {
			info.Current = "main (unborn)"
		}
	}

	rawList, err := c.Run("for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err == nil {
		for line := range strings.SplitSeq(rawList, "\n") {
			name := strings.TrimSpace(line)
			if name != "" {
				info.Branches = append(info.Branches, name)
			}
		}
	}
	if len(info.Branches) == 0 && info.Current != "" {
		info.Branches = []string{info.Current}
	}

	upstream, err := c.Run("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err == nil {
		info.Upstream = strings.TrimSpace(upstream)
		counts, countErr := c.Run("rev-list", "--left-right", "--count", "HEAD...@{u}")
		if countErr == nil {
			fields := strings.Fields(strings.TrimSpace(counts))
			if len(fields) == 2 {
				info.Ahead, _ = strconv.Atoi(fields[0])
				info.Behind, _ = strconv.Atoi(fields[1])
			}
		}
	}

	return info, nil
}

// ValidateBranchName 校验分支名称是否合法，保证前后端校验规则一致。
// 输入参数 name 为待校验的分支名；返回规范化后的分支名或校验错误。
func ValidateBranchName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("分支名称不能为空")
	}
	if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "/") || strings.HasSuffix(trimmed, "/") || strings.Contains(trimmed, "..") {
		return "", errors.New("分支名称格式不合法（不能以 - 或 / 开头结尾，也不能包含 ..）")
	}
	if !branchNameRegex.MatchString(trimmed) {
		return "", errors.New("分支名称仅允许包含字母、数字、点(.)、下划线(_)、斜杠(/)与连字符(-)")
	}
	return trimmed, nil
}

// CheckoutBranch 切换到指定的本地分支。
// 输入参数 branchName 为目标分支名，返回执行过程中的错误。
func (c *Client) CheckoutBranch(branchName string) error {
	validName, err := ValidateBranchName(branchName)
	if err != nil {
		return err
	}
	_, err = c.Run("checkout", validName)
	return err
}

// CreateAndCheckoutBranch 创建新分支并立即切换到该分支。
// 输入参数 branchName 为新分支名称，返回执行过程中的错误。
func (c *Client) CreateAndCheckoutBranch(branchName string) error {
	validName, err := ValidateBranchName(branchName)
	if err != nil {
		return err
	}
	_, err = c.Run("checkout", "-b", validName)
	return err
}

// Commit 提交当前已暂存（Staged）的更改；若 amend 为 true 则修正上一次提交。
// 输入参数 message 为提交说明文本，amend 为是否使用 --amend；返回执行错误。
func (c *Client) Commit(message string, amend bool) error {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return errors.New("提交信息 (Commit Message) 不能为空")
	}
	args := []string{"commit", "-m", msg}
	if amend {
		args = append(args, "--amend")
	}
	_, err := c.Run(args...)
	return err
}

// LastCommitMessage 获取最近一次提交的完整 Commit Message，用于勾选 Amend 时自动填充输入框。
// 无输入参数，返回最近一次提交消息字符串和可能的错误。
func (c *Client) LastCommitMessage() (string, error) {
	out, err := c.Run("log", "-1", "--format=%B")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Fetch 从默认远程仓库拉取最新引用信息（git fetch --all --prune）。
// 无输入参数，返回命令输出或错误。
func (c *Client) Fetch() (string, error) {
	return c.Run("fetch", "--all", "--prune")
}

// Pull 从当前分支关联的远程分支拉取并合并更新（git pull）。
// 无输入参数，返回命令输出或错误。
func (c *Client) Pull() (string, error) {
	return c.Run("pull")
}

// Push 将当前分支的提交推送到远程仓库（若尚未设置上游则自动带 -u origin <branch>）。
// 无输入参数，返回命令输出或错误。
func (c *Client) Push() (string, error) {
	info, err := c.GetBranchInfo()
	if err == nil && info.Upstream == "" && info.Current != "" && !strings.HasPrefix(info.Current, "HEAD") {
		return c.Run("push", "-u", "origin", info.Current)
	}
	return c.Run("push")
}
