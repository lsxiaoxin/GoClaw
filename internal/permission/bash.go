package permission

import (
	"path/filepath"
	"regexp"
	"strings"
)

var deniedBashPatterns = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{regexp.MustCompile(`(?i)(^|[\s;&|])(sudo)(\s|$)`), "privilege escalation is forbidden"},
	{regexp.MustCompile(`(?i)(^|[\s;&|])(shutdown|reboot|poweroff|halt)(\s|$)`), "system power commands are forbidden"},
	{regexp.MustCompile(`(?i)(^|[\s;&|])(mkfs(?:\.[a-z0-9]+)?|fdisk|parted)(\s|$)`), "disk formatting or partitioning is forbidden"},
	{regexp.MustCompile(`(?i)\bdd\b[^\n;]*\bof\s*=\s*/dev/`), "raw device writes are forbidden"},
	{regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;?\s*:`), "fork bombs are forbidden"},
}

// HardDenyBash returns a reason for commands that must never execute.
func HardDenyBash(command string) string {
	if recursivelyRemovesRoot(command) {
		return "recursive deletion of the filesystem root is forbidden"
	}
	for _, denied := range deniedBashPatterns {
		if denied.pattern.MatchString(command) {
			return denied.reason
		}
	}
	return ""
}

// ClearlyReadOnlyBash conservatively recognizes commands that only inspect state.
func ClearlyReadOnlyBash(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || strings.ContainsAny(command, "><;&`\n\r") ||
		strings.Contains(command, "$(") || strings.Contains(command, "||") {
		return false
	}

	segments := strings.Split(command, "|")
	for _, segment := range segments {
		fields := strings.Fields(strings.TrimSpace(segment))
		if len(fields) == 0 || !readOnlyCommand(fields) {
			return false
		}
	}
	return true
}

func readOnlyCommand(fields []string) bool {
	name := filepath.Base(strings.Trim(fields[0], `"'`))
	args := fields[1:]
	switch name {
	case "pwd", "ls", "tree", "cat", "head", "tail", "wc", "uniq", "cut",
		"tr", "grep", "rg", "ag", "awk", "du", "df", "stat", "file", "which",
		"whereis", "type", "realpath", "readlink", "basename", "dirname", "echo",
		"printf":
		return true
	case "find":
		return !containsAnyArgument(args,
			"-delete", "-exec", "-execdir", "-ok", "-okdir", "-fls", "-fprint", "-fprintf",
		)
	case "sed":
		return !hasOption(args, "-i", "--in-place")
	case "sort":
		return !hasOption(args, "-o", "--output")
	case "git":
		return readOnlyGit(args)
	case "go":
		return readOnlyGo(args)
	default:
		return false
	}
}

func readOnlyGit(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "status", "diff", "log", "show", "grep", "rev-parse", "ls-files",
		"ls-tree", "describe":
		return true
	default:
		return false
	}
}

func readOnlyGo(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "test", "vet", "list", "doc", "version":
		return true
	case "env":
		return !containsAnyArgument(args[1:], "-w", "-u")
	default:
		return false
	}
}

func containsAnyArgument(arguments []string, denied ...string) bool {
	for _, argument := range arguments {
		for _, value := range denied {
			if argument == value {
				return true
			}
		}
	}
	return false
}

func hasOption(arguments []string, short, long string) bool {
	for _, argument := range arguments {
		if argument == long || strings.HasPrefix(argument, long+"=") {
			return true
		}
		if argument == short ||
			(strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "--") &&
				strings.Contains(argument[1:], strings.TrimPrefix(short, "-"))) {
			return true
		}
	}
	return false
}

func recursivelyRemovesRoot(command string) bool {
	normalized := strings.NewReplacer(
		";", " ",
		"&&", " ",
		"||", " ",
		"|", " ",
		"\n", " ",
	).Replace(command)
	fields := strings.Fields(normalized)
	for index, field := range fields {
		name := strings.Trim(field, `"'`)
		if name != "rm" && !strings.HasSuffix(name, "/rm") {
			continue
		}

		var recursive, root bool
		for _, argument := range fields[index+1:] {
			argument = strings.Trim(argument, `"'`)
			if argument == "rm" || strings.HasSuffix(argument, "/rm") {
				break
			}
			if argument == "--no-preserve-root" || argument == "--recursive" {
				recursive = true
			}
			if strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "--") &&
				strings.Contains(strings.ToLower(argument), "r") {
				recursive = true
			}
			if argument == "/" || argument == "/*" {
				root = true
			}
		}
		if recursive && root {
			return true
		}
	}
	return false
}
