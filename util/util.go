package util

import (
	"strconv"
)

func Plural(count int, singular, plural string) string {
	if plural == "" {
		plural = singular + "s"
	}
	if count == 1 {
		return strconv.Itoa(count) + " " + singular
	}
	return strconv.Itoa(count) + " " + plural
}
