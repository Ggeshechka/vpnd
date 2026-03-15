//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var ifIndex string

func configureOutbound(o map[string]interface{}, physIP string) {
	// На Windows жестко привязываем исходящий трафик к физическому IP для обхода петли
	o["sendThrough"] = physIP
}

func getCmdOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

func setupNetwork() error {
	time.Sleep(3 * time.Second) // Wintun нужно время на создание адаптера

	idx, err := getCmdOutput("powershell", "-Command", "(Get-NetAdapter -Name 'xray0').InterfaceIndex")
	if err != nil || idx == "" {
		return fmt.Errorf("интерфейс xray0 не найден")
	}
	ifIndex = idx

	err = exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0", "0.0.0.0", "IF", ifIndex, "metric", "5").Run()
	if err != nil {
		return fmt.Errorf("ошибка добавления маршрута: %v", err)
	}

	return nil
}

func teardownNetwork() error {
	if ifIndex != "" {
		exec.Command("route", "delete", "0.0.0.0", "mask", "0.0.0.0", "0.0.0.0", "IF", ifIndex).Run()
	}
	return nil
}