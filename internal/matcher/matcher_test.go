package matcher

import (
	"reflect"
	"testing"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/model"
)

func TestNormalize(t *testing.T) {
	got := Normalize("  Pokémon: Écarlate & VIOLET (Switch) — cœur  ")
	want := []string{"pokemon", "ecarlate", "violet", "switch", "coeur"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		name   string
		filter model.TextFilter
		text   string
		want   bool
	}{
		{"empty filter accepts anything", model.TextFilter{}, "whatever", true},
		{"single word", inc("ps5"), "Jeu PS5 neuf", true},
		{"whole words only", inc("ps5"), "PS50 controller", false},
		{"accent insensitive", inc("pokemon"), "Pokémon Violet", true},
		{"accent in the rule", inc("écarlate"), "pokemon ecarlate", true},
		{"phrase in order", inc("god of war"), "God of War Ragnarök PS5", true},
		{"phrase words not consecutive", inc("god of war"), "God War of", false},
		{"punctuation ignored", inc("spider man"), "Marvel's Spider-Man 2", true},
		{"alternatives", inc("gow|god of war"), "GOW ragnarok", true},
		{"alternatives none", inc("gow|god of war"), "Horizon", false},
		{"all mode needs every term", inc("god of war", "ragnarok"), "God of War 2018", false},
		{"any mode needs one term", model.TextFilter{Include: []string{"zelda", "mario"}, IncludeMode: model.MatchAny}, "Mario Kart", true},
		{"prefix wildcard", inc("spider*"), "Spiderman PS4", true},
		{"suffix wildcard", inc("*phone"), "Iphone 13", true},
		{"contains wildcard", inc("*ider*"), "spiderman", true},
		{"quoted phrase", inc(`"jeux ps5"`), "Lot jeux PS5", true},
		{"regex", inc("/ps[45] (pro|slim)/"), "PS5 Slim digital", true},
		{"regex no match", inc("/ps[45] (pro|slim)/"), "PS5 digital", false},
		{"exclude wins", model.TextFilter{Include: []string{"ps5"}, Exclude: []string{"boite vide|boîte vide"}}, "PS5 boîte vide", false},
		{"exclude only", model.TextFilter{Exclude: []string{"lot"}}, "Lot de jeux", false},
		{"exclude only passes", model.TextFilter{Exclude: []string{"lot"}}, "Jeu seul", true},
		{"blank terms ignored", inc("  ", "|"), "anything", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Compile(tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			if got := f.Match(tc.text); got != tc.want {
				t.Fatalf("Match(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestInvalidRegex(t *testing.T) {
	if _, err := Compile(inc("/ps[5/")); err == nil {
		t.Fatal("expected an error for an invalid regular expression")
	}
}

func inc(terms ...string) model.TextFilter {
	return model.TextFilter{Include: terms, IncludeMode: model.MatchAll}
}
