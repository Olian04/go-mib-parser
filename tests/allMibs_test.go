package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mib_parser "github.com/Olian04/go-mib-parser"
)

func TestAllMibsParse(t *testing.T) {

	entries, err := os.ReadDir(filepath.Join("..", "mibs"))
	if err != nil {
		t.Fatalf("Failed to list mibs directory: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".mib" { // includes .mib and .MIB via ToLower
			continue
		}
		t.Run(name, func(t *testing.T) {
			mib, err := os.ReadFile(filepath.Join("..", "mibs", name))
			if err != nil {
				t.Fatalf("Failed to read %s: %v", name, err)
			}

			mod, err := mib_parser.ParseMIB(mib)
			if err != nil {
				t.Fatalf("Failed to parse %s: %v", name, err)
			}
			if mod == nil || mod.Name == "" {
				t.Fatalf("Parsed module from %s is empty", name)
			}
		})
	}
}
