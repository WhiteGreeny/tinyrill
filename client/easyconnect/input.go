package easyconnect

import (
	"errors"
	"regexp"
)

func requiredLoginField(data []byte, name string) (string, error) {
	match := regexp.MustCompile(`<` + name + `>([^<]*)</` + name + `>`).FindSubmatch(data)
	if len(match) != 2 || len(match[1]) == 0 || len(match[1]) > 4096 {
		return "", errors.New("missing or oversized EC login field: " + name)
	}
	return string(match[1]), nil
}
