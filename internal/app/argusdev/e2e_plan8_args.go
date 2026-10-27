package argusdev

import (
	"errors"
	"regexp"
	"strings"
)

func (check plan8Check) executionArgs() ([]string, error) {
	args := append([]string(nil), check.Args...)
	isTest := len(args) > 0 && args[0] == "test" || len(args) > 2 && args[0] == "-C" && args[2] == "test"
	if check.Program != "go" || !isTest {
		return args, nil
	}
	if len(check.Tests) == 0 {
		return nil, errors.New("Go acceptance check requires named tests")
	}
	names := make([]string, 0, len(check.Tests))
	for _, name := range check.Tests {
		if name == "" || strings.ContainsAny(name, "\r\n") {
			return nil, errors.New("invalid named acceptance test")
		}
		names = append(names, regexp.QuoteMeta(name))
	}
	pattern := "^(" + strings.Join(names, "|") + ")$"
	for i, arg := range args {
		if arg == "-run" {
			if i+1 >= len(args) {
				return nil, errors.New("missing Go test selector")
			}
			args[i+1] = pattern
			return args, nil
		}
		if strings.HasPrefix(arg, "-run=") {
			args[i] = "-run=" + pattern
			return args, nil
		}
	}
	return append(args, "-run", pattern), nil
}
