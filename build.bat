@echo off
setlocal

echo by zgcwkj
echo build start

rem config
set outPath=../build/markdownServer
cd ./src

rem windows_386
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=386
go build -ldflags="-w -s" -trimpath -o %outPath%_windows_386.exe

rem windows_amd64
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-w -s" -trimpath -o %outPath%_windows_amd64.exe

rem windows_arm64
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=arm64
go build -ldflags="-w -s" -trimpath -o %outPath%_windows_arm64.exe

rem darwin_amd64
set CGO_ENABLED=0
set GOOS=darwin
set GOARCH=amd64
go build -ldflags="-w -s" -trimpath -o %outPath%_darwin_amd64

rem darwin_arm64
set CGO_ENABLED=0
set GOOS=darwin
set GOARCH=arm64
go build -ldflags="-w -s" -trimpath -o %outPath%_darwin_arm64

rem linux_386
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=386
go build -ldflags="-w -s" -trimpath -o %outPath%_linux_386

rem linux_amd64
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-w -s" -trimpath -o %outPath%_linux_amd64

rem linux_arm
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=arm
go build -ldflags="-w -s" -trimpath -o %outPath%_linux_arm

rem linux_arm64
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=arm64
go build -ldflags="-w -s" -trimpath -o %outPath%_linux_arm64

rem linux_mips
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=mips
go build -ldflags="-w -s" -trimpath -o %outPath%_linux_mips

rem linux_mipsle
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=mipsle
go build -ldflags="-w -s" -trimpath -o %outPath%_linux_mipsle

rem linux_mips64
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=mips64
go build -ldflags="-w -s" -trimpath -o %outPath%_linux_mips64

rem linux_mips64le
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=mips64le
go build -ldflags="-w -s" -trimpath -o %outPath%_linux_mips64le

echo build success
pause
