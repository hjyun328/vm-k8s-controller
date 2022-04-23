package utils

import "strconv"

func DefaultString(val string, def string) string {
	if val == "" {
		return def
	}
	return val
}

func DefaultStringToInt(val string, def int) int {
	i, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return i
}
