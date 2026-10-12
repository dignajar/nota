package cli

import (
	"fmt"
	"strings"
)

// flagSpec declares one flag. Flags may appear anywhere among the
// positional arguments, which the standard flag package does not allow.
type flagSpec struct {
	name  string
	short string
	value bool   // takes a value
	multi bool   // may repeat
	help  string // shown in usage
	arg   string // value placeholder in usage
}

type parsed struct {
	vals  map[string][]string
	bools map[string]bool
	args  []string
}

func (p *parsed) str(name string) string {
	if v := p.vals[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

func (p *parsed) has(name string) bool {
	_, ok := p.vals[name]
	return ok || p.bools[name]
}

func parseArgs(args []string, specs []flagSpec) (*parsed, error) {
	p := &parsed{vals: map[string][]string{}, bools: map[string]bool{}}
	byName := map[string]flagSpec{}
	for _, s := range specs {
		byName["--"+s.name] = s
		if s.short != "" {
			byName["-"+s.short] = s
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			p.args = append(p.args, args[i+1:]...)
			break
		}
		// "-rf is dangerous" is note text: real flags never contain a space
		// unless written as --flag=value.
		isText := !strings.HasPrefix(a, "--") && strings.Contains(a, " ")
		if !strings.HasPrefix(a, "-") || a == "-" || isNumber(a) || isText {
			p.args = append(p.args, a)
			continue
		}
		key, val, hasVal := strings.Cut(a, "=")
		spec, ok := byName[key]
		if !ok {
			return nil, fmt.Errorf("unknown flag %s", key)
		}
		if !spec.value {
			if hasVal {
				return nil, fmt.Errorf("flag %s does not take a value", key)
			}
			p.bools[spec.name] = true
			continue
		}
		if !hasVal {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag %s needs a value", key)
			}
			i++
			val = args[i]
		}
		if !spec.multi && len(p.vals[spec.name]) > 0 {
			return nil, fmt.Errorf("flag --%s given twice", spec.name)
		}
		p.vals[spec.name] = append(p.vals[spec.name], val)
	}
	return p, nil
}

func isNumber(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}
