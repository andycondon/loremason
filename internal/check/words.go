package check

import "strings"

// Helpers for writing messages in plain words.

// typeNames are JSON's type names in plain words.
var typeNames = map[string]string{
	"array": "a list", "object": "a map", "string": "text", "integer": "a whole number",
	"number": "a number", "boolean": "true or false", "null": "empty",
}

// andList joins names as "a, b and c".
func andList(names []string) string { return joinLast(names, "and") }

// orList joins names as "a, b or c".
func orList(names []string) string { return joinLast(names, "or") }

func joinLast(names []string, last string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " " + last + " " + names[len(names)-1]
}

// a puts "a" or "an" before a word: a belief, an initiative.
func a(word string) string {
	if word != "" && strings.ContainsRune("aeiou", rune(word[0])) {
		return "an " + word
	}
	return "a " + word
}
