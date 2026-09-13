package acl

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

const (
	FlagGen uint32 = 1 << 31
	FlagSys uint32 = 1 << 15
	FlagCos uint32 = 1 << 14
	FlagGst uint32 = 1 << 13
	FlagPro uint32 = 1 << 12 // 見習い（オンライン登録直後。sysop 承認まで書けない）
	FlagAgt uint32 = 1 << 11 // エージェント（AgentIO。who で AI として分かる印）
)

type Command struct {
	Name  string
	Allow uint32
	Alias string
}

type Option struct {
	Name  string
	Allow uint32
}

type Flag struct {
	Name   string
	TLimit int
	Label  string
}

type Table struct {
	Commands []Command
	Options  []Option
	Flags    [32]Flag
}

func ParseMask(s string) uint32 {
	var v uint32
	n := 0
	for _, r := range s {
		switch r {
		case 'o', 'O':
			v = (v << 1) | 1
			n++
		case '-':
			v <<= 1
			n++
		}
	}
	if n < 32 {
		v <<= uint(32 - n)
	}
	return v
}

func Allowed(flags, allow uint32) bool {
	return flags&allow != 0
}

func (t *Table) LookupCommand(name string) (Command, bool) {
	name = strings.ToLower(name)
	var hit Command
	n := 0
	for _, c := range t.Commands {
		if c.Name == name {
			return c, true
		}
		if strings.HasPrefix(c.Name, name) {
			hit = c
			n++
		}
	}
	if n == 1 {
		return hit, true
	}
	return Command{}, false
}

func (t *Table) PrefixCommands(prefix string, flags uint32) []Command {
	prefix = strings.ToLower(prefix)
	var out []Command
	for _, c := range t.Commands {
		if strings.HasPrefix(c.Name, prefix) && Allowed(flags, c.Allow) {
			out = append(out, c)
		}
	}
	return out
}

func (t *Table) OptionOn(name string, flags uint32) bool {
	for _, o := range t.Options {
		if strings.EqualFold(o.Name, name) {
			return Allowed(flags, o.Allow)
		}
	}
	return false
}

func Load(etcDir string) (*Table, error) {
	t := &Table{}
	if err := t.loadCommands(etcDir + "/COMMAND.TXT"); err != nil {
		return nil, err
	}
	if err := t.loadOptions(etcDir + "/OPTION.TXT"); err != nil {
		return nil, err
	}
	if err := t.loadFlags(etcDir + "/FLAG.TXT"); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Table) loadCommands(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rest := splitWS(line)
		if name == "" {
			continue
		}
		mask, rest := splitWS(rest)
		t.Commands = append(t.Commands, Command{
			Name:  strings.ToLower(name),
			Allow: ParseMask(mask),
			Alias: strings.TrimSpace(rest),
		})
	}
	return sc.Err()
}

func (t *Table) loadOptions(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rest := splitWS(line)
		mask, _ := splitWS(rest)
		t.Options = append(t.Options, Option{
			Name:  strings.ToLower(name),
			Allow: ParseMask(mask),
		})
	}
	return sc.Err()
}

func (t *Table) loadFlags(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	i := 0
	for sc.Scan() && i < 32 {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rest := splitWS(line)
		lims, rest := splitWS(rest)
		lim := 0
		for _, r := range lims {
			if r < '0' || r > '9' {
				lim = 0
				break
			}
			lim = lim*10 + int(r-'0')
		}
		t.Flags[i] = Flag{Name: name, TLimit: lim, Label: rest}
		i++
	}
	return sc.Err()
}

func splitWS(s string) (string, string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	i := 0
	for i < len(s) && !unicode.IsSpace(rune(s[i])) {
		i++
	}
	if i >= len(s) {
		return s, ""
	}
	return s[:i], strings.TrimLeftFunc(s[i:], unicode.IsSpace)
}
