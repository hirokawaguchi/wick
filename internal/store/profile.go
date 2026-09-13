package store

import "strings"

const MaxScanList = 32

func ParseScanList(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key := strings.ToLower(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, line)
		if len(out) >= MaxScanList {
			break
		}
	}
	return out
}

func JoinScanList(items []string) string {
	return strings.Join(ParseScanList(strings.Join(items, "\n")), "\n")
}

func ApplyAutosign(body, sign string) string {
	sign = strings.TrimRight(sign, "\r\n")
	if strings.TrimSpace(sign) == "" {
		return body
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + "--\n" + sign + "\n"
}
