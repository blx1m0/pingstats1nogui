package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
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
	var cmd *exec.Cmd
	var ip string

	if runtime.GOOS == "windows" {
		// Для Windows используем ipconfig
		cmd = exec.Command("ipconfig")
	} else {
		// Для Linux используем ifconfig или ip a
		cmd = exec.Command("bash", "-c", "ifconfig | grep 'inet ' | grep -v 127.0.0.1 | awk '{print $2}'")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Ошибка при получении IP-адреса устройства: %v", err)
	}

	ip = strings.TrimSpace(string(output))
	if ip == "" {
		return "", fmt.Errorf("Не удалось найти IP-адрес устройства")
	}

	return ip, nil
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
	// Создание папки для логов, если её нет
	logDir := "stats_and_graphs"
	err := os.MkdirAll(logDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Ошибка при создании папки для логов: %v", err)
	}

	// Открываем лог-файл для записи (очищаем перед каждым запуском)
	logFile, err := os.OpenFile(logDir+"/ping_statistics.log", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Ошибка при открытии лог-файла: %v", err)
	}
	defer logFile.Close()

	// Настроим логирование в файл
	log.SetOutput(logFile)
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

	// Запускаем сбор данных в отдельной горутине
	go startPingCollection(hosts)

	// Запускаем GUI в основной горутине
	createGUI()
}
