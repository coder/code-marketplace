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

func ContainsCompare[T any](haystack []T, needle T, equal func(a, b T) bool) bool {
	for _, hay := range haystack {
		if equal(needle, hay) {
			return true
		}
	}
	return false
}

func Contains[T comparable](haystack []T, needle T) bool {
	return ContainsCompare(haystack, needle, func(a, b T) bool {
		return a == b
	})
}
