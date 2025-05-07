package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Функция для проверки и создания каталога логов
func ensureLogDir() error {
	logDir := "stats_and_graphs"
	if runtime.GOOS == "windows" {
		logDir = filepath.Join(".", logDir)
	}

	// Создаем каталог, если его нет
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		return fmt.Errorf("ошибка при создании каталога для логов: %v", err)
	}

	// Создаем файлы, если их нет
	files := []string{
		filepath.Join(logDir, "final_statistics.log"),
		filepath.Join(logDir, "ping_statistics.log"),
		filepath.Join(logDir, "mtr_results.log"),
	}

	for _, file := range files {
		if _, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err != nil {
			return fmt.Errorf("ошибка при создании файла %s: %v", file, err)
		}
	}

	return nil
}

// Функция для обновления статистики пинга в файле логов
func updatePingStats(host string, stats *PingStats) error {
	logDir := "stats_and_graphs"
	if runtime.GOOS == "windows" {
		logDir = filepath.Join(".", logDir)
	}

	// Открываем файл для добавления
	logFile := filepath.Join(logDir, "ping_statistics.log")
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("ошибка при открытии файла логов пинга: %v", err)
	}
	defer file.Close()

	// Записываем статистику
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	statsStr := fmt.Sprintf(
		"%s Хост: %s\n  Минимальное RTT: %.2f мс\n  Среднее RTT: %.2f мс\n  Максимальное RTT: %.2f мс\n  Потери пакетов: %.1f%%\n\n",
		timestamp,
		host,
		stats.MinRTT,
		stats.AvgRTT,
		stats.MaxRTT,
		stats.PacketLoss,
	)

	if _, err := file.WriteString(statsStr); err != nil {
		return fmt.Errorf("ошибка при записи статистики пинга: %v", err)
	}

	return nil
}

// Функция для обновления статистики MTR в файле логов
func updateMTRStats(host string, output string) error {
	logDir := "stats_and_graphs"
	if runtime.GOOS == "windows" {
		logDir = filepath.Join(".", logDir)
	}

	// Открываем файл для добавления
	logFile := filepath.Join(logDir, "mtr_results.log")
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("ошибка при открытии файла логов MTR: %v", err)
	}
	defer file.Close()

	// Записываем результаты
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	mtrStr := fmt.Sprintf("\n%s Результаты MTR до %s:\n%s\n", timestamp, host, output)

	if _, err := file.WriteString(mtrStr); err != nil {
		return fmt.Errorf("ошибка при записи результатов MTR: %v", err)
	}

	return nil
}
