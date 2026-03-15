//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"time"
)

func configureOutbound(o map[string]interface{}, physIP string) {
	if o["streamSettings"] == nil {
		o["streamSettings"] = make(map[string]interface{})
	}
	ss := o["streamSettings"].(map[string]interface{})

	if ss["sockopt"] == nil {
		ss["sockopt"] = make(map[string]interface{})
	}
	so := ss["sockopt"].(map[string]interface{})

	so["mark"] = 255
}

func setupNetwork() error {
	time.Sleep(3 * time.Second)
	
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
	exec.Command("ip", "rule", "del", "not", "fwmark", "255", "lookup", "100").Run()
	exec.Command("ip", "link", "delete", "dev", "xray0").Run()
	return nil
}