//go:build linux
package main

import (
	"fmt"
	"os/exec"
)

func setupNetwork() error {
	commands := [][]string{
		{"ip", "addr", "add", "172.19.0.1/30", "dev", "xray0"},
		{"ip", "link", "set", "dev", "xray0", "up"},
		{"ip", "rule", "add", "not", "fwmark", "255", "lookup", "100"},
		{"ip", "route", "add", "default", "dev", "xray0", "table", "100"},
	}

	for _, args := range commands {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			return fmt.Errorf("ошибка %v: %v", args, err)
		}
	}
	return nil
}

func teardownNetwork() error {
	return exec.Command("ip", "rule", "del", "not", "fwmark", "255", "lookup", "100").Run()
}