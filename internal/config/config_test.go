package config

import (
	"flag"
	"testing"
)

func TestParse(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	finish := Register(fs)
	if err := fs.Parse([]string{"-listen", "0.0.0.0:9000", "-data-dir", "somewhere"}); err != nil {
		t.Fatal(err)
	}
	c, err := finish()
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "0.0.0.0:9000" || c.IsLoopback() {
		t.Fatalf("unexpected config %+v", c)
	}
	if c.BrowserURL() != "http://127.0.0.1:9000" {
		t.Fatalf("unexpected browser URL %s", c.BrowserURL())
	}
}

func TestInvalidListen(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	finish := Register(fs)
	_ = fs.Parse([]string{"-listen", "8080"})
	if _, err := finish(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestIsLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8080": true,
		"localhost:1":    true,
		"[::1]:80":       true,
		"0.0.0.0:8080":   false,
		":8080":          false,
		"192.168.1.5:80": false,
	} {
		if got := (Config{Listen: addr}).IsLoopback(); got != want {
			t.Errorf("%s: got %v want %v", addr, got, want)
		}
	}
}
