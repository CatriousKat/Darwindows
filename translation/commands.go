package translation

import (
	"os"
	"strings"
)

func RemapCommand(cmd string) string {
	trimmed := strings.TrimSpace(cmd)
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return cmd
	}

	executable := strings.ToLower(parts[0])
	args := parts[1:]

	switch executable {
	case "open":
		if len(args) > 0 {
			return "start " + strings.Join(args, " ")
		}
		return "start ."
	case "ls":
		return "dir " + strings.Join(args, " ")
	case "cp":
		return "copy " + strings.Join(args, " ")
	case "mv":
		return "move " + strings.Join(args, " ")
	case "rm":
		return "del " + strings.Join(args, " ")
	case "clear":
		return "cls"
	case "which":
		return "where " + strings.Join(args, " ")
	}

	return cmd
}

func RemapPath(pathStr string) string {
	if strings.HasPrefix(pathStr, "/Users/") {
		parts := strings.SplitN(pathStr, "/", 4)
		if len(parts) >= 4 {
			return "C:\\Users\\" + strings.ReplaceAll(parts[3], "/", "\\")
		}
	} else if pathStr == "/Applications" || pathStr == "/Applications/" {
		return "C:\\Program Files"
	} else if strings.HasPrefix(pathStr, "/tmp") || strings.HasPrefix(pathStr, "/private/tmp") {
		sub := strings.TrimPrefix(strings.TrimPrefix(pathStr, "/private/tmp"), "/tmp")
		return os.TempDir() + strings.ReplaceAll(sub, "/", "\\")
	} else if pathStr == "~" || pathStr == "$HOME" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}

	return strings.ReplaceAll(pathStr, "/", "\\")
}

func GetMacVersion() string {
	return "macOS 10.15 Catalina"
}

func GetKernel() string {
	return "XNU"
}