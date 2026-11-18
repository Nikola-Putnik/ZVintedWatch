// Package matcher evaluates the keyword rules of an alert against listing
// titles and descriptions.
//
// Matching is case-insensitive and accent-insensitive ("Pokémon" matches
// "pokemon") and works on whole words, so "ps5" does not match "ps50".
// Each term of a rule supports a small syntax (see docs/keyword-syntax.md):
//
//	god of war        a phrase: consecutive words in this order
//	gow|god of war    alternatives: any of them
//	spider*           wildcard: words starting with "spider"
//	*phone            wildcard: words ending with "phone"
//	/ps[45] pro/      regular expression (matched against the normalized text)
//
// Include terms are combined with AND ("all") or OR ("any"); any exclude term
// rejects the text.
package matcher

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

// Filter is a compiled model.TextFilter. It is immutable and safe for
// concurrent use.
type Filter struct {
	include []group
	any     bool
	exclude []group
}

// group is one term: a set of alternatives separated by "|".
type group []alternative

type alternative interface {
	match(t *text) bool
}

// text is a normalized input, computed once per Match call.
type text struct {
	tokens []string
	joined string // tokens joined by single spaces
}

// Compile validates and compiles a filter.
func Compile(f model.TextFilter) (*Filter, error) {
	include, err := compileTerms(f.Include)
	if err != nil {
		return nil, err
	}
	exclude, err := compileTerms(f.Exclude)
	if err != nil {
		return nil, err
	}
	return &Filter{include: include, any: f.IncludeMode == model.MatchAny, exclude: exclude}, nil
}

// MustCompile is like Compile but panics on error. Intended for tests.
func MustCompile(f model.TextFilter) *Filter {
	c, err := Compile(f)
	if err != nil {
		panic(err)
	}
	return c
}

// IsEmpty reports whether the filter accepts every text.
func (f *Filter) IsEmpty() bool {
	return len(f.include) == 0 && len(f.exclude) == 0
}

// Match reports whether s satisfies the filter.
func (f *Filter) Match(s string) bool {
	if f.IsEmpty() {
		return true
	}
	t := newText(s)
	for _, g := range f.exclude {
		if g.match(t) {
			return false
		}
	}
	if len(f.include) == 0 {
		return true
	}
	if f.any {
		for _, g := range f.include {
			if g.match(t) {
				return true
			}
		}
		return false
	}
	for _, g := range f.include {
		if !g.match(t) {
			return false
		}
	}
	return true
}

func (g group) match(t *text) bool {
	for _, a := range g {
		if a.match(t) {
			return true
		}
	}
	return false
}

func compileTerms(terms []string) ([]group, error) {
	var out []group
	for _, term := range terms {
		g, err := compileTerm(term)
		if err != nil {
			return nil, err
		}
		if len(g) > 0 {
			out = append(out, g)
		}
	}
	return out, nil
}

func compileTerm(term string) (group, error) {
	term = strings.TrimSpace(term)
	if len(term) > 2 && strings.HasPrefix(term, "/") && strings.HasSuffix(term, "/") {
		re, err := regexp.Compile("(?i)" + term[1:len(term)-1])
		if err != nil {
			return nil, fmt.Errorf("invalid regular expression %s: %w", term, err)
		}
		return group{regexAlt{re}}, nil
	}
	var g group
	for _, alt := range strings.Split(term, "|") {
		if p := compilePhrase(alt); len(p) > 0 {
			g = append(g, p)
		}
	}
	return g, nil
}

// phrase is a sequence of word patterns that must appear consecutively.
type phrase []wordPattern

type wordPattern struct {
	word   string
	prefix bool // "word*": the token starts with word
	suffix bool // "*word": the token ends with word
}

func (w wordPattern) matches(token string) bool {
	switch {
	case w.prefix && w.suffix:
		return strings.Contains(token, w.word)
	case w.prefix:
		return strings.HasPrefix(token, w.word)
	case w.suffix:
		return strings.HasSuffix(token, w.word)
	default:
		return token == w.word
	}
}

func compilePhrase(s string) phrase {
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	var p phrase
	for _, piece := range strings.Fields(s) {
		leading := strings.HasPrefix(piece, "*")
		trailing := strings.HasSuffix(piece, "*")
		words := Normalize(strings.Trim(piece, "*"))
		for i, w := range words {
			p = append(p, wordPattern{
				word:   w,
				suffix: leading && i == 0,
				prefix: trailing && i == len(words)-1,
			})
		}
	}
	return p
}

func (p phrase) match(t *text) bool {
	n := len(p)
	for i := 0; i+n <= len(t.tokens); i++ {
		ok := true
		for j, w := range p {
			if !w.matches(t.tokens[i+j]) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

type regexAlt struct{ re *regexp.Regexp }

func (r regexAlt) match(t *text) bool { return r.re.MatchString(t.joined) }

func newText(s string) *text {
	tokens := Normalize(s)
	return &text{tokens: tokens, joined: strings.Join(tokens, " ")}
}

var ligatures = strings.NewReplacer("œ", "oe", "Œ", "oe", "æ", "ae", "Æ", "ae", "ß", "ss")

// Normalize lowercases s, removes accents and splits it into words made of
// letters and digits.
func Normalize(s string) []string {
	// A transformer keeps internal state, so build one per call.
	strip := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	plain, _, err := transform.String(strip, ligatures.Replace(s))
	if err != nil {
		plain = s
	}
	return strings.FieldsFunc(strings.ToLower(plain), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}
