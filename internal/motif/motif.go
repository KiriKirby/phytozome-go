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
		if c == '.' && i+2 < len(runes) && runes[i+1] == '.' && runes[i+2] == '.' {
			if len(tokens) == 0 || tokens[len(tokens)-1].kind != '*' {
				tokens = append(tokens, token{kind: '*'})
			}
			i += 3
			continue
		}
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
	runes := []rune(input)
	out := make([]token, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '.' && i+2 < len(runes) && runes[i+1] == '.' && runes[i+2] == '.' {
			if len(out) == 0 || out[len(out)-1].kind != '*' {
				out = append(out, token{kind: '*'})
			}
			i += 2
			continue
		}
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
	for start := 0; start <= len(runes); start++ {
		if matchFrom(tokens, 0, runes, start, make(map[matchState]int)) >= 0 {
			return true
		}
	}
	return false
}

type matchState struct {
	tokenIndex int
	position   int
}

func matchFrom(tokens []token, ti int, text []rune, pos int, memo map[matchState]int) int {
	if ti >= len(tokens) {
		return pos
	}
	state := matchState{tokenIndex: ti, position: pos}
	if result, ok := memo[state]; ok {
		return result
	}
	t := tokens[ti]
	if t.kind == '*' {
		for next := pos; next <= len(text); next++ {
			if end := matchFrom(tokens, ti+1, text, next, memo); end >= 0 {
				memo[state] = end
				return end
			}
		}
		memo[state] = -1
		return -1
	}
	if pos >= len(text) {
		memo[state] = -1
		return -1
	}
	if t.kind == 'a' {
		for _, candidate := range t.alternates {
			if next := matchCandidate(candidate, text, pos); next >= 0 {
				if end := matchFrom(tokens, ti+1, text, next, memo); end >= 0 {
					memo[state] = end
					return end
				}
			}
		}
		memo[state] = -1
		return -1
	}
	next := matchCandidate([]token{t}, text, pos)
	if next < 0 {
		memo[state] = -1
		return -1
	}
	result := matchFrom(tokens, ti+1, text, next, memo)
	memo[state] = result
	return result
}

func matchCandidate(tokens []token, text []rune, pos int) int {
	if len(tokens) == 0 {
		return pos
	}
	t := tokens[0]
	if t.kind == '*' {
		for next := pos; next <= len(text); next++ {
			if end := matchCandidate(tokens[1:], text, next); end >= 0 {
				return end
			}
		}
		return -1
	}
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
	return matchCandidate(tokens[1:], text, pos+1)
}
