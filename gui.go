package main

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type PingStats struct {
	Host       string
	MinRTT     float64
	MaxRTT     float64
	AvgRTT     float64
	PacketLoss float64
	LastUpdate time.Time
}

var (
	statsMap      = make(map[string]*PingStats)
	statsMutex    sync.RWMutex
	mainWindow    fyne.Window
	interval      int      = 10
	systemHosts   []string // Хосты, собранные при запуске
	extraHosts    []string // Дополнительные хосты
	stopTicker    chan bool
	mtrRunning    bool
	mtrStopChan   chan bool
	mtrWindow     fyne.Window      // Окно для MTR
	mtrTextWidget *widget.TextGrid // Виджет для вывода MTR
	mtrEntry      *widget.Entry    // Поле ввода для MTR в главном окне
	defaultHosts  = []string{
		"8.8.8.8",    // Google DNS
		"1.1.1.1",    // Cloudflare DNS
		"77.88.8.8",  // Yandex DNS
		"ya.ru",      // Yandex
		"google.com", // Google
		"github.com", // GitHub
	}
)

func updateStatsTable(table *widget.Table) {
	fyne.Do(func() {
		table.Refresh()
	})
}

func showStatistics() {
	statsMutex.RLock()
	defer statsMutex.RUnlock()

	statsWindow := fyne.CurrentApp().NewWindow("Статистика пинга")
	statsWindow.Resize(fyne.NewSize(800, 600))

	text := "Собранная статистика:\n\n"
	for host, stats := range statsMap {
		text += fmt.Sprintf("Хост: %s\n", host)
		text += fmt.Sprintf("  Минимальное RTT: %.2f мс\n", stats.MinRTT)
		text += fmt.Sprintf("  Среднее RTT: %.2f мс\n", stats.AvgRTT)
		text += fmt.Sprintf("  Максимальное RTT: %.2f мс\n", stats.MaxRTT)
		text += fmt.Sprintf("  Потери пакетов: %.1f%%\n", stats.PacketLoss)
		text += fmt.Sprintf("  Последнее обновление: %s\n\n", stats.LastUpdate.Format("15:04:05"))
	}

	textWidget := widget.NewTextGrid()
	textWidget.SetText(text)

	scrollContainer := container.NewScroll(textWidget)
	statsWindow.SetContent(scrollContainer)
	statsWindow.Show()
}

func runMTRAndUpdateWindow(host string) {
	if mtrRunning {
		return
	}

	mtrRunning = true
	mtrStopChan = make(chan bool)

	go func() {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			if !checkCommandAvailable("tracert") {
				fyne.Do(func() {
					if mtrTextWidget != nil {
						mtrTextWidget.SetText("Ошибка: tracert не найден в системе.\n")
					}
					mtrRunning = false
				})
				return
			}
			cmd = exec.Command("tracert", host)
		} else {
			if !checkCommandAvailable("mtr") {
				fyne.Do(func() {
					if mtrTextWidget != nil {
						mtrTextWidget.SetText("Ошибка: mtr не найден в системе.\n")
					}
					mtrRunning = false
				})
				return
			}
			cmd = exec.Command("mtr", "-n", "-r", "-c", "1", host)
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			fyne.Do(func() {
				if mtrTextWidget != nil {
					mtrTextWidget.SetText(fmt.Sprintf("Ошибка трассировки: %v\n", err))
				}
				mtrRunning = false
			})
			return
		}
		if err := ensureLogDir(); err != nil {
			log.Printf("Ошибка при создании каталога для логов: %v", err)
		} else {
			if err := updateMTRStats(host, string(output)); err != nil {
				log.Printf("Ошибка при обновлении файла логов MTR: %v", err)
			}
		}
		fyne.Do(func() {
			if mtrTextWidget != nil {
				mtrTextWidget.SetText(string(output))
			}
			mtrRunning = false
		})
	}()
}

func showMTRStats() {
	if mtrWindow != nil {
		mtrWindow.Show()
		mtrWindow.Canvas().Refresh(mtrWindow.Content())
		return
	}
	mtrWindow = fyne.CurrentApp().NewWindow("Статистика MTR")
	mtrWindow.Resize(fyne.NewSize(800, 600))
	mtrTextWidget = widget.NewTextGrid()
	scrollContainer := container.NewScroll(mtrTextWidget)
	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder("Введите хост для трассировки")
	if mtrEntry != nil {
		hostEntry.SetText(mtrEntry.Text)
	}
	var stopButton *widget.Button
	stopButton = widget.NewButton("Остановить трассировку", func() {
		if mtrRunning {
			mtrStopChan <- true
			mtrRunning = false
			fyne.Do(func() {
				stopButton.Disable()
			})
		}
	})
	stopButton.Disable()
	startButton := widget.NewButton("Запустить трассировку", func() {
		host := hostEntry.Text
		if host == "" {
			return
		}
		if mtrEntry != nil {
			mtrEntry.SetText(host)
		}
		runMTRAndUpdateWindow(host)
		stopButton.Enable()
	})
	controls := container.NewVBox(
		hostEntry,
		container.NewHBox(startButton, stopButton),
	)
	content := container.NewBorder(controls, nil, nil, nil, scrollContainer)
	mtrWindow.SetContent(content)
	mtrWindow.SetOnClosed(func() {
		mtrWindow = nil
		mtrTextWidget = nil
	})
	if runtime.GOOS == "windows" {
		mtrTextWidget.SetText("Внимание: на Windows используется tracert, а не mtr!\n")
	}
	mtrWindow.Show()
	mtrWindow.Canvas().Refresh(mtrWindow.Content())
}

func createGUI(initialHosts []string) {
	myApp := app.New()
	mainWindow = myApp.NewWindow("Ping Statistics")
	mainWindow.Resize(fyne.NewSize(1200, 800))

	// Сохраняем системные хосты
	systemHosts = initialHosts

	// Создаем темную тему
	myApp.Settings().SetTheme(&customTheme{})

	// Создаем таблицу для статистики
	statsTable := widget.NewTable(
		func() (int, int) {
			return len(statsMap) + 1, 5 // +1 для заголовка
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.TextStyle = fyne.TextStyle{Monospace: true}
			return container.NewPadded(label)
		},
		func(i widget.TableCellID, o fyne.CanvasObject) {
			container := o.(*fyne.Container)
			label := container.Objects[0].(*widget.Label)

			if i.Row == 0 {
				// Заголовки
				headers := []string{"Хост", "Мин. RTT", "Макс. RTT", "Ср. RTT", "Потери"}
				label.SetText(headers[i.Col])
				label.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				statsMutex.RLock()
				defer statsMutex.RUnlock()

				// Данные
				hosts := make([]string, 0, len(statsMap))
				for h := range statsMap {
					hosts = append(hosts, h)
				}
				if i.Row-1 < len(hosts) {
					stats := statsMap[hosts[i.Row-1]]
					switch i.Col {
					case 0:
						label.SetText(fmt.Sprintf("%-20s", stats.Host))
					case 1:
						label.SetText(fmt.Sprintf("%10.2f ms", stats.MinRTT))
					case 2:
						label.SetText(fmt.Sprintf("%10.2f ms", stats.MaxRTT))
					case 3:
						label.SetText(fmt.Sprintf("%10.2f ms", stats.AvgRTT))
					case 4:
						label.SetText(fmt.Sprintf("%8.1f%%", stats.PacketLoss))
					}
				}
			}
		},
	)

	// Устанавливаем фиксированную ширину колонок
	statsTable.SetColumnWidth(0, 200) // Хост
	statsTable.SetColumnWidth(1, 120) // Мин. RTT
	statsTable.SetColumnWidth(2, 120) // Макс. RTT
	statsTable.SetColumnWidth(3, 120) // Ср. RTT
	statsTable.SetColumnWidth(4, 100) // Потери

	// Создаем элементы управления
	intervalEntry := widget.NewEntry()
	intervalEntry.SetText("10")
	intervalEntry.Validator = func(s string) error {
		val := 0
		_, err := fmt.Sscanf(s, "%d", &val)
		if err != nil || val < 5 || val > 3600 {
			return fmt.Errorf("интервал должен быть от 5 до 3600 секунд")
		}
		return nil
	}

	hostsEntry := widget.NewEntry()
	hostsEntry.SetPlaceHolder("Введите дополнительные хосты через запятую")

	// Добавляем отдельное поле для MTR
	mtrEntry = widget.NewEntry()
	mtrEntry.SetPlaceHolder("Введите хост для MTR")
	mtrEntry.Validator = func(s string) error {
		if s == "" {
			return fmt.Errorf("хост не может быть пустым")
		}
		return nil
	}

	stopTicker = make(chan bool)

	// Создаем кнопки
	startButton := widget.NewButton("Запустить пинг", func() {
		// Проверяем и создаем каталог для логов перед запуском
		if err := ensureLogDir(); err != nil {
			log.Printf("Ошибка при создании каталога для логов: %v", err)
		}

		interval = 10
		if val, err := fmt.Sscanf(intervalEntry.Text, "%d", &interval); err == nil && val > 0 {
			// Валидация уже выполнена в Validator
		}

		// Собираем все хосты: системные + дополнительные
		allHosts := make([]string, len(systemHosts))
		copy(allHosts, systemHosts)

		// Добавляем дополнительные хосты, если они есть
		if hostsEntry.Text != "" {
			extraHosts = strings.Split(hostsEntry.Text, ",")
			for _, host := range extraHosts {
				host = strings.TrimSpace(host)
				if host != "" {
					allHosts = append(allHosts, host)
				}
			}
		}

		go func() {
			ticker := time.NewTicker(time.Duration(interval) * time.Second)
			defer ticker.Stop()

			// Создаем таймер для автоматической остановки
			stopTimer := time.NewTimer(time.Duration(interval) * time.Second)
			defer stopTimer.Stop()

			// Сразу запускаем первый сбор статистики
			startPingCollection(allHosts)
			updateStatsTable(statsTable)

			for {
				select {
				case <-ticker.C:
					startPingCollection(allHosts)
					updateStatsTable(statsTable)
				case <-stopTimer.C:
					// По истечении интервала отправляем сигнал остановки
					// Проверяем и создаем каталог для логов перед остановкой
					if err := ensureLogDir(); err != nil {
						log.Printf("Ошибка при создании каталога для логов: %v", err)
					}

					select {
					case stopTicker <- true:
						// Сигнал отправлен
						// Обновляем содержимое каталога после остановки
						if err := updateLogDir(); err != nil {
							log.Printf("Ошибка при обновлении каталога логов: %v", err)
						}
					default:
						// Канал не готов, значит пинг уже остановлен
					}
					return
				case <-stopTicker:
					return
				}
			}
		}()
	})

	stopButton := widget.NewButton("Остановить", func() {
		// Проверяем и создаем каталог для логов перед остановкой
		if err := ensureLogDir(); err != nil {
			log.Printf("Ошибка при создании каталога для логов: %v", err)
		}

		select {
		case stopTicker <- true:
			// Сигнал отправлен
			// Создаем канал для синхронизации
			done := make(chan bool)

			// Обновляем логи в горутине
			go func() {
				if err := updateLogDir(); err != nil {
					log.Printf("Ошибка при обновлении каталога логов: %v", err)
				}
				done <- true
			}()

			// Ждем завершения обновления логов
			<-done
		default:
			// Канал не готов, значит пинг не запущен
		}
	})

	showStatsButton := widget.NewButton("Показать статистику", showStatistics)
	showMTRStatsButton := widget.NewButton("Показать статистику MTR", showMTRStats)

	exitButton := widget.NewButton("Выход", func() {
		mainWindow.Close()
	})

	mtrButton := widget.NewButton("Запустить MTR", func() {
		host := mtrEntry.Text
		if host == "" {
			return
		}
		if mtrWindow == nil {
			showMTRStats()
		}
		runMTRAndUpdateWindow(host)
	})

	// Создаем подпись автора
	authorLabel := canvas.NewText("Made by Lg$", color.RGBA{255, 165, 0, 255})
	authorLabel.TextSize = 14

	// Создаем контейнер с элементами управления
	controls := container.NewVBox(
		widget.NewLabel("Интервал (сек):"),
		intervalEntry,
		widget.NewLabel("Хосты для пинга (через запятую):"),
		hostsEntry,
		container.NewHBox(startButton, stopButton),
		widget.NewLabel("Хост для MTR:"),
		mtrEntry,
		container.NewHBox(mtrButton),
		container.NewHBox(showStatsButton, showMTRStatsButton, exitButton),
	)

	// Создаем контейнер с отступами
	content := container.NewBorder(
		container.NewPadded(controls),
		container.NewPadded(container.NewHBox(authorLabel)),
		nil, nil,
		container.NewPadded(statsTable),
	)

	mainWindow.SetContent(content)
	mainWindow.ShowAndRun()
	mainWindow.Canvas().Refresh(mainWindow.Content())
}

// Кастомная тема
type customTheme struct{}

func (t *customTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{33, 33, 33, 255} // Темный фон
	case theme.ColorNameForeground:
		return color.RGBA{255, 165, 0, 255} // Оранжевый текст
	case theme.ColorNameButton:
		return color.RGBA{66, 66, 66, 255} // Темно-серые кнопки
	case theme.ColorNameHover:
		return color.RGBA{255, 165, 0, 128} // Полупрозрачный оранжевый при наведении
	case theme.ColorNameInputBackground:
		return color.RGBA{44, 44, 44, 255} // Темный фон для полей ввода
	case theme.ColorNameInputBorder:
		return color.RGBA{255, 165, 0, 255} // Оранжевая рамка для полей ввода
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (t *customTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *customTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *customTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}

// Функция для обновления содержимого каталога логов
func updateLogDir() error {
	logDir := "stats_and_graphs"
	if runtime.GOOS == "windows" {
		logDir = filepath.Join(".", logDir)
	}

	// Создаем каталог, если его нет
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		return fmt.Errorf("ошибка при создании каталога для логов: %v", err)
	}

	// Создаем или обновляем файл с итоговой статистикой
	statsFile := filepath.Join(logDir, "final_statistics.log")
	file, err := os.OpenFile(statsFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("ошибка при открытии файла статистики: %v", err)
	}
	defer file.Close()

	// Записываем текущую статистику
	statsMutex.RLock()
	defer statsMutex.RUnlock()

	file.WriteString("Итоговая статистика пинга:\n\n")
	for host, stats := range statsMap {
		file.WriteString(fmt.Sprintf("Хост: %s\n", host))
		file.WriteString(fmt.Sprintf("  Минимальное RTT: %.2f мс\n", stats.MinRTT))
		file.WriteString(fmt.Sprintf("  Среднее RTT: %.2f мс\n", stats.AvgRTT))
		file.WriteString(fmt.Sprintf("  Максимальное RTT: %.2f мс\n", stats.MaxRTT))
		file.WriteString(fmt.Sprintf("  Потери пакетов: %.1f%%\n", stats.PacketLoss))
		file.WriteString(fmt.Sprintf("  Последнее обновление: %s\n\n", stats.LastUpdate.Format("2006/01/02 15:04:05")))
	}

	return nil
}
