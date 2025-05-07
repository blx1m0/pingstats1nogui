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
		results <- fmt.Sprintf("Ошибка при пинге %s: %v", host, err)
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
	updateStatsMap(host, stats)

	results <- fmt.Sprintf("Результаты пинга для %s:\n%s", host, output)
}

// Функция для парсинга статистики пинга
func parsePingStats(output, host string) *PingStats {
	stats := &PingStats{
		Host:       host,
		LastUpdate: time.Now(),
	}

	// Регулярные выражения для извлечения статистики
	minRTTRe := regexp.MustCompile(`min/avg/max.*?=.*?(\d+\.?\d*)/(\d+\.?\d*)/(\d+\.?\d*)`)
	lossRe := regexp.MustCompile(`(\d+)% packet loss`)

	// Ищем минимальное, среднее и максимальное время
	if matches := minRTTRe.FindStringSubmatch(output); len(matches) > 3 {
		stats.MinRTT, _ = strconv.ParseFloat(matches[1], 64)
		stats.AvgRTT, _ = strconv.ParseFloat(matches[2], 64)
		stats.MaxRTT, _ = strconv.ParseFloat(matches[3], 64)
	}

	// Ищем процент потери пакетов
	if matches := lossRe.FindStringSubmatch(output); len(matches) > 1 {
		stats.PacketLoss, _ = strconv.ParseFloat(matches[1], 64)
	}

	return stats
}

// Функция для обновления карты статистики
func updateStatsMap(host string, stats *PingStats) {
	statsMap[host] = stats
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
func getFirstThreeHops(traceOutput string) []string {
	lines := strings.Split(traceOutput, "\n")
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
	return hops
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
		cmd = exec.Command("mtr", "-n", "-c", "1", "-m", strconv.Itoa(maxHops), host)
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
	// Канал для сбора результатов пинга
	results := make(chan string, len(hosts))
	var wg sync.WaitGroup

	// Пинг каждого хоста параллельно
	for _, host := range hosts {
		wg.Add(1)
		go pingHost(host, &wg, results)
	}

	// Ждем завершения всех горутин
	wg.Wait()
	close(results)

	// Выводим все результаты пинга в лог
	for result := range results {
		log.Println(result)
	}

	log.Println("------------")
	log.Println("Завершено выполнение программы.")
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

	// Создание папки для логов с учетом ОС
	logDir := "stats_and_graphs"
	if runtime.GOOS == "windows" {
		logDir = filepath.Join(".", logDir)
	}
	err := os.MkdirAll(logDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Ошибка при создании папки для логов: %v", err)
	}

	// Открываем лог-файл с учетом ОС
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

	// Получаем IP-адрес устройства
	deviceIP, err := getDeviceIP()
	if err != nil {
		log.Fatalf("Ошибка при получении IP-адреса устройства: %v", err)
	}
	log.Printf("IP адрес устройства: %s\n", deviceIP)

	// Получаем первые 3 хопа до 8.8.8.8
	traceResult, err := traceRoute("8.8.8.8")
	if err != nil {
		log.Fatalf("Ошибка при трассировке до 8.8.8.8: %v", err)
	}

	// Извлекаем первые 3 хопа
	hops := getFirstThreeHops(traceResult)
	log.Printf("Трассировка до 8.8.8.8 (первые 3 хопа):\n")
	for i, hop := range hops {
		log.Printf("  %d. %s\n", i+1, hop)
	}

	// Формируем список хостов для пинга
	hosts := []string{
		deviceIP,    // IP устройства
		"127.0.0.1", // Локальный адрес
		"77.88.8.8", // Публичный DNS Google
		"80.250.224.3",
		"80.250.226.3",
		"10.1.1.2", // Пример IP для шлюза
	}
	// Добавляем хопы из трассировки
	hosts = append(hosts, hops...)

	// Запрашиваем дополнительный хост для пинга
	fmt.Print("\nВведите дополнительный хост для пинга (или нажмите Enter для пропуска): ")
	var additionalHost string
	fmt.Scanln(&additionalHost)
	if additionalHost != "" {
		hosts = append(hosts, additionalHost)
	}

	// Запрашиваем интервал тестирования
	fmt.Print("\nВведите интервал тестирования в секундах (от 5 до 3600): ")
	var interval int
	fmt.Scanln(&interval)
	if interval < 5 {
		interval = 5
	} else if interval > 3600 {
		interval = 3600
	}

	// Запрашиваем параметры для MTR
	fmt.Print("\nХотите запустить MTR? (y/n): ")
	var runMTRChoice string
	fmt.Scanln(&runMTRChoice)
	if strings.ToLower(runMTRChoice) == "y" {
		fmt.Print("Введите хост для MTR: ")
		var mtrHost string
		fmt.Scanln(&mtrHost)
		if mtrHost != "" {
			fmt.Print("Введите максимальное количество хопов (1-30): ")
			var maxHops int
			fmt.Scanln(&maxHops)
			if maxHops < 1 {
				maxHops = 1
			} else if maxHops > 30 {
				maxHops = 30
			}
			err := runMTR(mtrHost, maxHops)
			if err != nil {
				log.Printf("Ошибка при запуске MTR: %v", err)
			}
		}
	}

	// Запускаем сбор данных с указанным интервалом
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				startPingCollection(hosts)
			}
		}
	}()

	fmt.Printf("\nЗапущен сбор статистики с интервалом %d секунд.\n", interval)
	fmt.Println("Нажмите Enter для завершения...")
	fmt.Scanln()
	done <- true
}
