@echo off
echo Building Windows executable...

set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=1

go build -o pingstats.exe

echo Done!
pause 