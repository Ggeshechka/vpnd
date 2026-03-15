package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"	

	"github.com/kardianos/service"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
	_ "github.com/xtls/xray-core/main/json"
)

const (
	vpsIP   = "188.40.167.82"
	tunName = "xray0"
)

var xrayInstance *core.Instance

type program struct{}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) run() {
	http.HandleFunc("/start", apiStart)
	http.HandleFunc("/stop", apiStop)
	log.Println("Служба запущена. API слушает на 127.0.0.1:18080")
	http.ListenAndServe("127.0.0.1:18080", nil)
}

func (p *program) Stop(s service.Service) error {
	stopVPN()
	return nil
}

func getPhysicalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

func startVPN(configData []byte) error {
	if xrayInstance != nil {
		return fmt.Errorf("VPN уже запущен")
	}

	physIP, err := getPhysicalIP()
	if err != nil {
		return err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		return err
	}

	outbounds := config["outbounds"].([]interface{})
	for _, out := range outbounds {
		o := out.(map[string]interface{})
		tag := o["tag"].(string)
		if tag == "proxy" || tag == "direct" {
			configureOutbound(o, physIP)
		}
	}

	newConfigBytes, _ := json.Marshal(config)
	pbConfig, err := serial.DecodeJSONConfig(bytes.NewReader(newConfigBytes))
	if err != nil {
		return err
	}

	cfg, err := pbConfig.Build()
	if err != nil {
		return err
	}

	server, err := core.New(cfg)
	if err != nil {
		return err
	}

	if err := server.Start(); err != nil {
		return err
	}
	xrayInstance = server

	if err := setupNetwork(); err != nil {
		stopVPN()
		return fmt.Errorf("ошибка маршрутизации: %v", err)
	}

	return nil
}

func stopVPN() {
	if xrayInstance != nil {
		teardownNetwork()
		xrayInstance.Close()
		xrayInstance = nil
		time.Sleep(1 * time.Second) // Даем ОС время освободить ресурсы
	}
}

func apiStart(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	// Если тело пустое (отправили без конфига), читаем локальный config.json
	if err != nil || len(body) == 0 {
		body, err = os.ReadFile("config.json")
		if err != nil {
			http.Error(w, "Конфиг не передан и файл config.json не найден", http.StatusBadRequest)
			return
		}
	}

	if err := startVPN(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("vpn_started"))
}

func apiStop(w http.ResponseWriter, r *http.Request) {
	stopVPN()
	w.Write([]byte("vpn_stopped"))
	
	// Очищаем состояние Wintun через смерть процесса
	// Служба будет мгновенно перезапущена системой
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(1)
	}()
}



func main() {
	exe, _ := os.Executable()
	os.Setenv("xray.location.asset", filepath.Dir(exe))

	svcConfig := &service.Config{
		Name:        "vpnd",
		DisplayName: "VPN Daemon",
		Description: "Фоновая служба для управления ядром VPN",
		Option: service.KeyValue{
			"OnFailure": "restart", // <--- ДОБАВИТЬ ЭТО. Заставит Windows перезапускать процесс
		},
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	// Обработка команд (install, uninstall, start, stop, restart)
	if len(os.Args) > 1 {
		err = service.Control(s, os.Args[1])
		if err != nil {
			log.Fatalf("Ошибка выполнения команды %s: %v", os.Args[1], err)
		}
		fmt.Printf("Команда %s успешно выполнена.\n", os.Args[1])
		return
	}

	// Запуск самой службы
	if err = s.Run(); err != nil {
		log.Fatal(err)
	}
}