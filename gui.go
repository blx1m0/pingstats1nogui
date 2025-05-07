package main

import (
	"fmt"
	"image/color"
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
	statsMap   = make(map[string]*PingStats)
	statsMutex sync.RWMutex
	mainWindow fyne.Window
)

func createGUI() {
	myApp := app.New()
	mainWindow = myApp.NewWindow("Ping Statistics")
	mainWindow.Resize(fyne.NewSize(1000, 600))

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

	// Создаем кнопку обновления
	updateButton := widget.NewButton("Обновить статистику", func() {
		statsTable.Refresh()
	})

	// Создаем подпись автора
	authorLabel := canvas.NewText("Made by Lg$", color.RGBA{255, 165, 0, 255})
	authorLabel.TextSize = 14

	// Создаем контейнер с отступами
	content := container.NewBorder(
		container.NewPadded(container.NewHBox(updateButton)),
		container.NewPadded(container.NewHBox(authorLabel)),
		nil, nil,
		container.NewPadded(statsTable),
	)

	mainWindow.SetContent(content)

	// Запускаем автоматическое обновление каждые 5 секунд
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			statsTable.Refresh()
		}
	}()

	mainWindow.ShowAndRun()
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
