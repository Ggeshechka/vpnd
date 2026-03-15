//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"time"
)

func configureOutbound(o map[string]interface{}, physIP string) {
	o["sendThrough"] = physIP
}

func setupNetwork() error {
	time.Sleep(3 * time.Second) // Wintun нужно время на создание адаптера

	// Назначаем IP-адрес интерфейсу
	err := exec.Command("netsh", "interface", "ip", "set", "address", "name=xray0", "source=static", "addr=172.19.0.2", "mask=255.255.255.0").Run()
	if err != nil {
		return fmt.Errorf("ошибка назначения IP адаптеру xray0: %v", err)
	}

	// Направляем весь трафик в TUN
	err = exec.Command("netsh", "interface", "ipv4", "add", "route", "0.0.0.0/0", "xray0", "172.19.0.2", "metric=5").Run()
	if err != nil {
		return fmt.Errorf("ошибка добавления маршрута: %v", err)
	}

	return nil
}

func teardownNetwork() error {
	exec.Command("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/0", "xray0", "172.19.0.2").Run()
	return nil
}