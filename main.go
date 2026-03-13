package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
	_ "github.com/xtls/xray-core/main/json"
)

type XrayManager struct {
	server *core.Instance
}

func (m *XrayManager) Start(configJSON string) error {
	if m.server != nil {
		return errors.New("xray уже запущен")
	}

	pbConfig, err := serial.DecodeJSONConfig(bytes.NewReader([]byte(configJSON)))
	if err != nil {
		return fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	config, err := pbConfig.Build()
	if err != nil {
		return fmt.Errorf("ошибка сборки конфига: %w", err)
	}

	server, err := core.New(config)
	if err != nil {
		return fmt.Errorf("ошибка инициализации: %w", err)
	}

	if err := server.Start(); err != nil {
		return fmt.Errorf("ошибка запуска Xray: %w", err)
	}
	m.server = server

	log.Println("Настройка маршрутизации...")
	if err := setupNetwork(); err != nil {
		m.Stop()
		return fmt.Errorf("ошибка настройки сети: %w", err)
	}

	return nil
}

func (m *XrayManager) Stop() error {
	if m.server == nil {
		return errors.New("xray не запущен")
	}

	log.Println("Восстановление маршрутизации...")
	_ = teardownNetwork()

	err := m.server.Close()
	m.server = nil
	return err
}

func main() {
	os.Setenv("xray.location.asset", ".")

	configBytes, err := os.ReadFile("config.json")
	if err != nil {
		log.Fatalf("Ошибка чтения config.json: %v", err)
	}

	manager := &XrayManager{}

	if err := manager.Start(string(configBytes)); err != nil {
		log.Fatalf("Критическая ошибка: %v", err)
	}
	log.Println("TUN поднят. Нажмите Ctrl+C для выхода.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Остановка...")
	manager.Stop()
	log.Println("Завершено.")
}