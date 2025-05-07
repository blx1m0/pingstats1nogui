package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// Функция для пинга адреса с использованием системной утилиты ping
func pingHost(host string, wg *sync.WaitGroup, results chan<- string) {
	defer wg.Done()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "4", "-w", "1000", host)
	} else {
		cmd = exec.Command("ping", "-c", "4", "-W", "1", host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Создаем статистику с ошибкой
		stats := &PingStats{
			Host:       host,
			MinRTT:     0,
			MaxRTT:     0,
			AvgRTT:     0,
			PacketLoss: 100, // 100% потерь при ошибке
			LastUpdate: time.Now(),
		}
		updateStatsMap(host, stats)
		results <- fmt.Sprintf("Ошибка при пинге %s: %v\n%s", host, err, output)
		return
	}

	// Конвертируем вывод в UTF-8 для Windows
	if runtime.GOOS == "windows" {
		decoder := charmap.Windows1251.NewDecoder()
		reader := transform.NewReader(bytes.NewReader(output), decoder)
		output, err = io.ReadAll(reader)
		if err != nil {
			results <- fmt.Sprintf("Ошибка при конвертации кодировки для %s: %v", host, err)
			return
		}
	}

	// Парсим статистику
	stats := parsePingStats(string(output), host)
	if stats.PacketLoss == 100 {
		// Если все пакеты потеряны, устанавливаем время в 0
		stats.MinRTT = 0
		stats.MaxRTT = 0
		stats.AvgRTT = 0
	}
	updateStatsMap(host, stats)

	results <- fmt.Sprintf("Результаты пинга для %s:\n%s", host, output)
}

// Функция для парсинга статистики пинга
func parsePingStats(output, host string) *PingStats {
	stats := &PingStats{
		Host:       host,
		LastUpdate: time.Now(),
		PacketLoss: 100, // По умолчанию считаем, что все пакеты потеряны
	}

	// Регулярные выражения для извлечения статистики
	minRTTRe := regexp.MustCompile(`min/avg/max.*?=.*?(\d+\.?\d*)/(\d+\.?\d*)/(\d+\.?\d*)`)
	lossRe := regexp.MustCompile(`(\d+)% packet loss`)
	transmittedRe := regexp.MustCompile(`(\d+) packets transmitted, (\d+) received`)

	// Ищем информацию о переданных и полученных пакетах
	if matches := transmittedRe.FindStringSubmatch(output); len(matches) > 2 {
		transmitted, _ := strconv.Atoi(matches[1])
		received, _ := strconv.Atoi(matches[2])
		if transmitted > 0 {
			stats.PacketLoss = float64(transmitted-received) * 100 / float64(transmitted)
		}
	}

	// Ищем минимальное, среднее и максимальное время
	if matches := minRTTRe.FindStringSubmatch(output); len(matches) > 3 {
		stats.MinRTT, _ = strconv.ParseFloat(matches[1], 64)
		stats.AvgRTT, _ = strconv.ParseFloat(matches[2], 64)
		stats.MaxRTT, _ = strconv.ParseFloat(matches[3], 64)
	} else if matches := lossRe.FindStringSubmatch(output); len(matches) > 1 {
		// Если не нашли RTT, но нашли потери пакетов
		stats.PacketLoss, _ = strconv.ParseFloat(matches[1], 64)
	}

	return stats
}

// Функция для обновления карты статистики
func updateStatsMap(host string, stats *PingStats) {
	statsMutex.Lock()
	defer statsMutex.Unlock()
	statsMap[host] = stats

	// Обновляем статистику в файле
	if err := updatePingStats(host, stats); err != nil {
		log.Printf("Ошибка при обновлении файла статистики: %v", err)
	}
}

// Функция для трассировки маршрута до хоста
func traceRoute(host string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("tracert", host)
	} else {
		cmd = exec.Command("traceroute", host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Ошибка при трассировке: %v", err)
	}
	return string(output), nil
}

// Функция для извлечения первых 3 хопов из вывода traceroute
func getFirstThreeHops(host string) ([]string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("tracert", "-h", "3", host)
	} else {
		cmd = exec.Command("traceroute", "-m", "3", host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ошибка при трассировке: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	hops := []string{}
	// Регулярное выражение для извлечения IP-адресов из скобок
	re := regexp.MustCompile(`\((\d+\.\d+\.\d+\.\d+)\)`)

	for i, line := range lines {
		if i >= 3 { // Нам нужны только первые 3 хопа
			break
		}
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			hops = append(hops, matches[1]) // Добавляем IP хопа
		}
	}

	if len(hops) == 0 {
		return nil, fmt.Errorf("не удалось получить хопы")
	}

	return hops, nil
}

// Функция для получения IP-адреса устройства в Linux и Windows
func getDeviceIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("Ошибка при получении сетевых интерфейсов: %v", err)
	}

	for _, iface := range ifaces {
		// Пропускаем неактивные интерфейсы и loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			// Преобразуем адрес в IP
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Пропускаем IPv6 и локальные адреса
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}

			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("Не удалось найти IP-адрес устройства")
}

// Функция для проверки доступности утилиты
func checkCommandAvailable(cmd string) bool {
	var checkCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		checkCmd = exec.Command("where", cmd)
	} else {
		checkCmd = exec.Command("which", cmd)
	}
	return checkCmd.Run() == nil
}

// Функция для запуска MTR до указанного хоста
func runMTR(host string, maxHops int) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		if !checkCommandAvailable("tracert") {
			return fmt.Errorf("утилита tracert не найдена в системе")
		}
		cmd = exec.Command("tracert", "-h", strconv.Itoa(maxHops), host)
	} else {
		if !checkCommandAvailable("mtr") {
			return fmt.Errorf("утилита mtr не найдена в системе. Установите её с помощью: sudo apt-get install mtr")
		}
		cmd = exec.Command("mtr", "-n", "-r", "-c", "1", "-m", strconv.Itoa(maxHops), host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Ошибка при запуске MTR: %v", err)
	}

	// Конвертируем вывод в UTF-8 для Windows
	if runtime.GOOS == "windows" {
		decoder := charmap.Windows1251.NewDecoder()
		reader := transform.NewReader(bytes.NewReader(output), decoder)
		output, err = io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("Ошибка при конвертации кодировки: %v", err)
		}
	}

	log.Printf("Результаты MTR до %s:\n%s", host, string(output))
	return nil
}

func startPingCollection(hosts []string) {
	if len(hosts) == 0 {
		log.Println("Не указаны хосты для пинга")
		return
	}

	// Канал для сбора результатов пинга
	results := make(chan string, len(hosts))
	var wg sync.WaitGroup

	// Пинг каждого хоста параллельно
	for _, host := range hosts {
		if host == "" {
			continue
		}
		wg.Add(1)
		go pingHost(host, &wg, results)
	}

	// Запускаем горутину для закрытия канала после завершения всех пингов
	go func() {
		wg.Wait()
		close(results)
	}()

	// Читаем результаты из канала
	for result := range results {
		log.Println(result)
	}

	log.Println("------------")
	log.Println("Завершено выполнение программы.")
}

// Функция для сбора информации о сети
func collectNetworkInfo() ([]string, error) {
	var hosts []string

	// Получаем IP устройства
	deviceIP, err := getDeviceIP()
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении IP устройства: %v", err)
	}
	hosts = append(hosts, deviceIP)
	log.Printf("IP адрес устройства: %s", deviceIP)

	// Получаем шлюз по умолчанию
	gateway, err := getDefaultGateway()
	if err != nil {
		log.Printf("Предупреждение: не удалось получить шлюз по умолчанию: %v", err)
	} else {
		hosts = append(hosts, gateway)
		log.Printf("Шлюз по умолчанию: %s", gateway)
	}

	// Получаем первые 3 хопа до 8.8.8.8
	hops, err := getFirstThreeHops("8.8.8.8")
	if err != nil {
		log.Printf("Предупреждение: не удалось получить хопы до 8.8.8.8: %v", err)
	} else {
		hosts = append(hosts, hops...)
		log.Printf("Первые 3 хопа до 8.8.8.8: %v", hops)
	}

	// Добавляем стандартные DNS-серверы
	hosts = append(hosts, "8.8.8.8", "1.1.1.1", "77.88.8.8")

	return hosts, nil
}

// Функция для получения шлюза по умолчанию
func getDefaultGateway() (string, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("ipconfig")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", err
		}

		// Конвертируем вывод в UTF-8 для Windows
		decoder := charmap.Windows1251.NewDecoder()
		reader := transform.NewReader(bytes.NewReader(output), decoder)
		output, err = io.ReadAll(reader)
		if err != nil {
			return "", err
		}

		// Ищем строку с Default Gateway
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Default Gateway") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					gateway := strings.TrimSpace(parts[1])
					if gateway != "" && gateway != "0.0.0.0" {
						return gateway, nil
					}
				}
			}
		}
	} else {
		cmd := exec.Command("ip", "route", "show", "default")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", err
		}

		// Ищем IP-адрес после "via"
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "via") {
				parts := strings.Split(line, "via")
				if len(parts) > 1 {
					gateway := strings.Fields(parts[1])[0]
					if gateway != "" && gateway != "0.0.0.0" {
						return gateway, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("шлюз по умолчанию не найден")
}

func main() {
	// Инициализация кодировки для Windows
	if runtime.GOOS == "windows" {
		// Устанавливаем кодировку консоли в UTF-8
		cmd := exec.Command("chcp", "65001")
		cmd.Run()
	}

	// Проверяем доступность необходимых утилит
	if !checkCommandAvailable("ping") {
		log.Fatal("Утилита ping не найдена в системе")
	}

	// Создание папки для логов
	if err := ensureLogDir(); err != nil {
		log.Fatalf("Ошибка при создании папки для логов: %v", err)
	}

	// Открываем лог-файл с учетом ОС
	logDir := "stats_and_graphs"
	if runtime.GOOS == "windows" {
		logDir = filepath.Join(".", logDir)
	}
	logPath := filepath.Join(logDir, "ping_statistics.log")
	logFile, err := os.OpenFile(logPath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Ошибка при открытии лог-файла: %v", err)
	}
	defer logFile.Close()

	// Настроим логирование в файл и в консоль
	log.SetOutput(io.MultiWriter(logFile, os.Stdout))
	log.Println("Программа для сбора статистики пинга!")
	log.Println("Лог сохранён в stats_and_graphs/ping_statistics.log")
	log.Println("Made by Lg$")

	// Собираем информацию о сети
	networkHosts, err := collectNetworkInfo()
	if err != nil {
		log.Printf("Предупреждение: %v", err)
	}

	// Запускаем GUI с собранными хостами
	createGUI(networkHosts)
}
