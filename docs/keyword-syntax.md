# Keyword syntax

An alert can have keyword rules for the listing **title** and for its **description**. Each set of
rules has two lists:

- **Must contain** — the terms that must appear. Choose **All terms** (every term is required) or
  **Any term** (one is enough).
- **Must not contain** — if any of these terms appears, the listing is rejected.

Empty lists mean “no restriction”. An alert without any rule notifies every new listing of its
search.

## How text is compared

Before comparing, both the rule and the listing text are normalized:

- **case is ignored**: `PS5` = `ps5`;
- **accents are ignored**: `Pokémon` = `pokemon`, `écarlate` = `ecarlate`
  (`œ` → `oe`, `æ` → `ae`, `ß` → `ss`);
- **punctuation separates words**: `Spider-Man` = `spider man`, `PS5/PS4` = `ps5 ps4`.

Terms match **whole words**: `ps5` matches “Jeu PS5” but not “PS50”.

## Term syntax

| You write | Meaning | Matches | Does not match |
|---|---|---|---|
| `ragnarok` | one word | “God of War **Ragnarök**” | “Ragnaroks” |
| `god of war` | a phrase: these words, consecutive, in this order | “**God of War** PS5” | “War god of” |
| `gow\|god of war` | alternatives: any of them | “**GOW** Ragnarok”, “**God of War**” | “Horizon” |
| `spider*` | words **starting** with “spider” | “**Spiderman**”, “**Spider**-Man” | “Myspider” |
| `*phone` | words **ending** with “phone” | “**iPhone** 13” | “phones” |
| `*ider*` | words **containing** “ider” | “**Spider**” | — |
| `"jeux ps5"` | quotes are optional and ignored | “Lot **jeux PS5**” | — |
| `/ps[45] (pro\|slim)/` | regular expression | “**PS5 Slim** digital” | “PS5 digital” |

In the editor, press **Enter** (or type a comma) to add a term. Use the **Try a listing title**
box to check your rules instantly.

### Regular expressions

A term written between slashes is a [Go regular expression](https://pkg.go.dev/regexp/syntax),
matched case-insensitively against the **normalized** text: lowercase, without accents, words
separated by single spaces. Write your expression accordingly (`/ragnarok/`, not `/Ragnarök/`).
Commas inside a regular expression do not split the term.

## Examples

**Only God of War games for PS5, no empty boxes**

- Title must contain (all): `god of war|gow`
- Title must not contain: `ps4`, `boite vide`, `empty box`

**Any of several games (one alert instead of three)**

- Title must contain (**any**): `zelda`, `mario kart`, `smash bros`

**Sealed items only — checked in the description**

- Description must contain (any): `neuf|sealed|blister|scellé`
- Description must not contain: `ouvert|opened`

**Sneakers in a size range, excluding kids**

- Search URL with brand and size filters from Vinted
- Title must not contain: `enfant`, `kids`, `junior`

## Description rules and requests

On Vinted, search results do not include descriptions, so reading one costs one extra request.
To keep requests low, ZVintedWatch only reads the description of a **new** listing whose title and
price already satisfy at least one alert that has description rules. Each description is read at
most once, then shared by every alert. Prefer title rules whenever possible.

## Price range

The optional price range is checked by ZVintedWatch itself, not by the marketplace. This lets
several alerts with different budgets share the same search (and therefore the same request).
If you always want the same price limit, you can also put it in the search URL (`price_to=…`).
