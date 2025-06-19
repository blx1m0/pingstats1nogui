# PingStats

Кросс-платформенная утилита для мониторинга сетевой статистики.

## Возможности

- Пинг нескольких хостов одновременно
- Трассировка маршрута (MTR/traceroute, на Windows используется tracert)
- Настраиваемый интервал тестирования
- Логирование результатов
- Поддержка Windows и Linux

## Требования

### Windows
- Go 1.16 или выше
- Встроенные утилиты: ping, tracert (для трассировки)

### Linux
- Go 1.16 или выше
- Утилита mtr: `sudo apt-get install mtr` (для Ubuntu/Debian, только для Linux)
- Встроенные утилиты: ping, traceroute

## Установка

1. Клонируйте репозиторий:
```bash
git clone https://github.com/yourusername/pingstats.git
cd pingstats
```

2. Скомпилируйте программу:
```bash
# Для Windows
go build -o pingstats.exe

# Для Linux
go build -o pingstats
```

## Использование

1. Запустите программу:
```bash
# Windows
.\pingstats.exe

# Linux
./pingstats
```

2. Следуйте инструкциям в консоли:
   - Введите дополнительный хост для пинга (опционально)
   - Укажите интервал тестирования (5-3600 секунд)
   - Выберите, нужно ли запускать MTR
   - При выборе MTR укажите хост и количество хопов

## Логи

Результаты сохраняются в директории `stats_and_graphs/ping_statistics.log`

## Лицензия

MIT 