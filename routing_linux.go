//go:build linux
package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func getDefaultGateway() (string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) >= 3 && fields[0] == "default" {
		return fields[2], nil
	}
	return "", fmt.Errorf("шлюз не найден")
}

func setupRouting() error {
	time.Sleep(3 * time.Second)

	gateway, err := getDefaultGateway()
	if err != nil {
		return err
	}

	cmds := [][]string{
		{"ip", "addr", "add", "172.19.0.2/24", "dev", tunName},
		{"ip", "link", "set", "dev", tunName, "up"},
		{"ip", "route", "add", vpsIP, "via", gateway},
		{"ip", "route", "add", "0.0.0.0/1", "dev", tunName},
		{"ip", "route", "add", "128.0.0.0/1", "dev", tunName},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if err := cmd.Run(); err != nil {
			fmt.Printf("Ошибка при выполнении %v: %v\n", args, err)
		}
	}

	return nil
}