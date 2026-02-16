package llm

import (
	"strings"
)

func ProcessMessage(content string) string {
	reversed := reverseString(content)
	return reversed
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func ProcessMessageWithPrefix(content string, prefix string) string {
	reversed := reverseString(content)
	return prefix + ": " + reversed
}

func ProcessMessageWordReverse(content string) string {
	words := strings.Fields(content)
	for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
		words[i], words[j] = words[j], words[i]
	}
	return strings.Join(words, " ")
}
