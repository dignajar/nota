//go:build darwin

package service

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestPlistIsWellFormed(t *testing.T) {
	p := Plist("/Users/me/bin/nota & co", "/Users/me/.local/share/nota")
	d := xml.NewDecoder(strings.NewReader(p))
	d.Strict = true
	for {
		_, err := d.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("invalid plist: %v\n%s", err, p)
		}
	}
	if !strings.Contains(p, "nota &amp; co") || !strings.Contains(p, "<key>KeepAlive</key>") {
		t.Fatalf("unexpected plist:\n%s", p)
	}
}
