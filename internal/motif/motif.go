package motif

import (
	"strconv"
	"strings"
)

type token struct {
	kind       byte
	literal    rune
	alternates [][]token
}

func SplitList(input string) []string {
	parts := strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == '，' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func MatchingIndexes(sequence string, motifs []string) []int {
	sequence = strings.ToUpper(strings.TrimSpace(sequence))
	out := make([]int, 0, len(motifs))
	for index, raw := range motifs {
		if tokens := parse(raw); len(tokens) > 0 && contains(tokens, sequence) {
			out = append(out, index+1)
		}
	}
	return out
}

// AllMatched reports whether matches contains every 1-based motif index exactly once.
func AllMatched(matches string, motifCount int) bool {
	if motifCount <= 0 || strings.TrimSpace(matches) == "" {
		return false
	}
	seen := make([]bool, motifCount)
	for _, raw := range strings.Split(matches, ",") {
		index, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || index < 1 || index > motifCount || seen[index-1] {
			return false
		}
		seen[index-1] = true
	}
	for _, matched := range seen {
		if !matched {
			return false
		}
	}
	return true
}

func parse(input string) []token {
	runes := []rune(strings.TrimSpace(input))
	tokens := make([]token, 0, len(runes))
	for i := 0; i < len(runes); {
		c := runes[i]
		switch c {
		case 'X', 'x':
			tokens = append(tokens, token{kind: 'x'})
			i++
			continue
		case '-', '－', '−':
			tokens = append(tokens, token{kind: '-'})
			i++
			continue
		}
		close := rune(0)
		switch c {
		case '[':
			close = ']'
		case '【':
			close = '】'
		case '［':
			close = '］'
		case '(':
			close = ')'
		case '（':
			close = '）'
		}
		if close != 0 {
			end := i + 1
			for end < len(runes) && runes[end] != close {
				end++
			}
			if end < len(runes) {
				body := strings.NewReplacer("／", "/", "、", "/").Replace(string(runes[i+1 : end]))
				raw := []string{}
				if strings.Contains(body, "/") {
					raw = strings.Split(body, "/")
				} else {
					for _, r := range []rune(body) {
						raw = append(raw, string(r))
					}
				}
				alternates := make([][]token, 0, len(raw))
				for _, candidate := range raw {
					if parsed := parseCandidate(strings.TrimSpace(candidate)); len(parsed) > 0 {
						alternates = append(alternates, parsed)
					}
				}
				if len(alternates) > 0 {
					tokens = append(tokens, token{kind: 'a', alternates: alternates})
					i = end + 1
					continue
				}
			}
		}
		tokens = append(tokens, token{kind: 'l', literal: []rune(strings.ToUpper(string(c)))[0]})
		i++
	}
	return tokens
}

func parseCandidate(input string) []token {
	out := make([]token, 0, len([]rune(input)))
	for _, r := range []rune(input) {
		switch r {
		case 'X', 'x':
			out = append(out, token{kind: 'x'})
		case '-', '－', '−':
			out = append(out, token{kind: '-'})
		default:
			out = append(out, token{kind: 'l', literal: []rune(strings.ToUpper(string(r)))[0]})
		}
	}
	return out
}

func contains(tokens []token, text string) bool {
	runes := []rune(text)
	for start := range runes {
		if matchFrom(tokens, 0, runes, start) >= 0 {
			return true
		}
	}
	return false
}

func matchFrom(tokens []token, ti int, text []rune, pos int) int {
	if ti >= len(tokens) {
		return pos
	}
	if pos >= len(text) {
		return -1
	}
	t := tokens[ti]
	if t.kind == 'a' {
		for _, candidate := range t.alternates {
			if next := matchCandidate(candidate, text, pos); next >= 0 {
				if end := matchFrom(tokens, ti+1, text, next); end >= 0 {
					return end
				}
			}
		}
		return -1
	}
	next := matchCandidate([]token{t}, text, pos)
	if next < 0 {
		return -1
	}
	return matchFrom(tokens, ti+1, text, next)
}

func matchCandidate(tokens []token, text []rune, pos int) int {
	for _, t := range tokens {
		if pos >= len(text) {
			return -1
		}
		current := text[pos]
		switch t.kind {
		case 'x':
			if current == '-' || current == '.' {
				return -1
			}
		case '-':
			if current != '-' && current != '.' {
				return -1
			}
		case 'l':
			if current != t.literal {
				return -1
			}
		}
		pos++
	}
	return pos
}
