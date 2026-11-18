package vinted

import "sort"

// domains maps every supported Vinted country domain to the Accept-Language
// header a local browser would send.
var domains = map[string]string{
	"vinted.at":    "de-AT,de;q=0.9,en;q=0.8",
	"vinted.be":    "fr-BE,fr;q=0.9,nl;q=0.8,en;q=0.7",
	"vinted.co.uk": "en-GB,en;q=0.9",
	"vinted.com":   "en-US,en;q=0.9",
	"vinted.cz":    "cs-CZ,cs;q=0.9,en;q=0.8",
	"vinted.de":    "de-DE,de;q=0.9,en;q=0.8",
	"vinted.dk":    "da-DK,da;q=0.9,en;q=0.8",
	"vinted.es":    "es-ES,es;q=0.9,en;q=0.8",
	"vinted.fi":    "fi-FI,fi;q=0.9,en;q=0.8",
	"vinted.fr":    "fr-FR,fr;q=0.9,en;q=0.8",
	"vinted.gr":    "el-GR,el;q=0.9,en;q=0.8",
	"vinted.hr":    "hr-HR,hr;q=0.9,en;q=0.8",
	"vinted.hu":    "hu-HU,hu;q=0.9,en;q=0.8",
	"vinted.ie":    "en-IE,en;q=0.9",
	"vinted.it":    "it-IT,it;q=0.9,en;q=0.8",
	"vinted.lt":    "lt-LT,lt;q=0.9,en;q=0.8",
	"vinted.lu":    "fr-LU,fr;q=0.9,de;q=0.8,en;q=0.7",
	"vinted.nl":    "nl-NL,nl;q=0.9,en;q=0.8",
	"vinted.pl":    "pl-PL,pl;q=0.9,en;q=0.8",
	"vinted.pt":    "pt-PT,pt;q=0.9,en;q=0.8",
	"vinted.ro":    "ro-RO,ro;q=0.9,en;q=0.8",
	"vinted.se":    "sv-SE,sv;q=0.9,en;q=0.8",
	"vinted.sk":    "sk-SK,sk;q=0.9,en;q=0.8",
}

func domainList() []string {
	out := make([]string, 0, len(domains))
	for d := range domains {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func acceptLanguage(domain string) string {
	if v, ok := domains[domain]; ok {
		return v
	}
	return "en-US,en;q=0.9"
}
