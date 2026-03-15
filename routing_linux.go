//go:build linux
package main

import (
	"fmt"
	"os/exec"
)

func setupNetwork() error {
	// Поднимаем интерфейс и даем ему IP
	commands := [][]string{
		{"ip", "addr", "add", "172.19.0.1/30", "dev", "xray0"},
		{"ip", "link", "set", "dev", "xray0", "up"},
	}

	for _, args := range commands {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			return fmt.Errorf("ошибка %v: %v", args, err)
		}
	}

	// Настраиваем маршрутизацию через fwmark
	routeCmds := [][]string{
		// Создаем дефолтный маршрут в TUN в отдельной таблице 100
		{"ip", "route", "replace", "default", "dev", "xray0", "table", "100"},
		
		// Правило 1: Трафик, исходящий от самого Xray (маркированный 255), идет в обход TUN (в основную таблицу)
		{"ip", "rule", "add", "fwmark", "255", "lookup", "main", "pref", "1000"},
		
		// Правило 2: Весь остальной трафик направляем в таблицу 100 (в TUN)
		{"ip", "rule", "add", "lookup", "100", "pref", "1010"},
	}

	for _, args := range routeCmds {
		// Ошибки тут не фатальны (например, правило уже существует), поэтому просто логируем
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			fmt.Printf("предупреждение при выполнении %v: %v\n", args, err)
		}
	}

	return nil
}

func teardownNetwork() error {
	// Удаляем правила при выходе
	exec.Command("ip", "rule", "del", "fwmark", "255", "lookup", "main", "pref", "1000").Run()
	exec.Command("ip", "rule", "del", "lookup", "100", "pref", "1010").Run()
	exec.Command("ip", "route", "flush", "table", "100").Run()
	return nil
}