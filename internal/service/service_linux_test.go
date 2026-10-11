//go:build linux

package service

import (
	"strings"
	"testing"
)

func TestUnit(t *testing.T) {
	u := Unit("/home/me/My Tools/nota", "/home/me/.local/share/nota")
	for _, want := range []string{
		`ExecStart="/home/me/My Tools/nota" daemon`,
		"Environment=NOTA_HOME=/home/me/.local/share/nota",
		"WantedBy=default.target",
		"Restart=on-failure",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("unit missing %q:\n%s", want, u)
		}
	}
}
