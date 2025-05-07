@echo off
echo Building Windows GUI executable...

set CGO_ENABLED=1
go build -ldflags "-H windowsgui" -o pingstats_gui.exe

echo Done!
pause 