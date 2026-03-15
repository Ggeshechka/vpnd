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

func setupRouting() error {
	time.Sleep(3 * time.Second)

	cmds := [][]string{
		{"ip", "addr", "add", "172.19.0.2/24", "dev", tunName},
		{"ip", "link", "set", "dev", tunName, "up"},
		{"ip", "route", "add", "default", "dev", tunName, "table", "100"},
		{"ip", "rule", "add", "fwmark", "255", "lookup", "main"},
		{"ip", "rule", "add", "not", "fwmark", "255", "table", "100"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if err := cmd.Run(); err != nil {
			fmt.Printf("Ошибка при выполнении %v: %v\n", args, err)
		}
	}

	return nil
}