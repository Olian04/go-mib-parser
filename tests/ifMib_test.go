package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	mib_parser "github.com/Olian04/go-mib-parser"
)

func TestIfMib(t *testing.T) {
	mib, err := os.ReadFile(filepath.Join("..", "mibs", "IF-MIB.MIB"))
	if err != nil {
		t.Fatalf("Failed to read IF-MIB: %v", err)
	}

	ifMib, err := mib_parser.ParseMIB(mib)
	if err != nil {
		t.Fatalf("Failed to parse IF-MIB: %v", err)
	}
	ifMib.GetObjectByName("ifIndex")

	fmt.Println(ifMib)
}
