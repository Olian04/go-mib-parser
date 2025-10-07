package main

import (
	"fmt"
	"os"

	parser "github.com/Olian04/go-mib-parser/parser"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: debug <path-to-mib>")
		os.Exit(1)
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	ir, err := parser.Parse(b)
	if err != nil {
		panic(err)
	}
	fmt.Println("Module:", ir.Name)
	fmt.Println("ObjectIdentities:")
	for k := range ir.ObjectIdentities {
		fmt.Println(" -", k)
	}
	fmt.Println("ObjectTypes:")
	for k := range ir.ObjectsByName {
		fmt.Println(" -", k)
	}
	targets := []string{"ifNumber", "ifIndex", "ifName", "ifRcvAddressAddress", "ifTestId", "ifStackHigherLayer"}
	for _, n := range targets {
		if _, ok := ir.ObjectsByName[n]; !ok {
			fmt.Println("MISSING:", n)
		}
	}
	fmt.Println("Nodes:")
	for k := range ir.NodesByName {
		if k == "zeroDotZero" {
			fmt.Println(" has zeroDotZero node")
		}
	}
	for k := range ir.NodesByName {
		fmt.Println(k)
	}
	for _, n := range targets {
		if _, ok := ir.NodesByName[n]; ok {
			fmt.Println("NODE HAS:", n)
		}
	}
	fmt.Println("Notifications:")
	for k := range ir.NotificationTypes {
		fmt.Println(" -", k)
	}
}
