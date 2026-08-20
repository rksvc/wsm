//go:build windows

package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

//go:embed dist
var dist embed.FS

var Exe string

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	Exe = exe
}

func main() {
	if isService, err := svc.IsWindowsService(); err != nil {
		log.Fatal(err)
	} else if isService {
		const WSM = "WSM"
		eventlog.InstallAsEventCreate(WSM, eventlog.Error|eventlog.Warning|eventlog.Info)
		elog, err := eventlog.Open(WSM)
		if err != nil {
			log.Fatal(err)
		}
		defer elog.Close()
		if err := svc.Run(WSM, &Handler{elog: elog}); err != nil {
			elog.Error(1000, err.Error())
		}
		return
	}

	var addr string
	flag.StringVar(&addr, "addr", "127.0.0.1:3483", "address to run server on")
	flag.Parse()

	if !windows.GetCurrentProcessToken().IsElevated() {
		if err := elevate(); err != nil {
			log.Fatal(err)
		}
		return
	}

	s, err := New()
	if err != nil {
		log.Fatal(err)
	}

	dist, err := fs.Sub(dist, "dist")
	if err != nil {
		log.Fatal(err)
	}
	e := echo.New()
	e.Use(middleware.Gzip())
	e.Use(middleware.RequestLogger())
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(dist))))
	api := e.Group("/api")
	api.GET("/services", s.Services)
	api.POST("/services", s.InstallService)
	api.POST("/services/:name", s.EditService)
	api.GET("/services/:name/config", s.ServiceConfig)
	api.GET("/services/:name/processes", s.ServiceProcesses)
	api.PUT("/services/:name/start", s.StartService)
	api.PUT("/services/:name/stop", s.StopService)
	api.PUT("/services/:name/restart", s.RestartService)
	api.DELETE("/services/:name", s.DeleteService)

	if err := e.Start(addr); err != nil {
		log.Fatal(err)
	}
}

func elevate() error {
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(Exe)
	if err != nil {
		return err
	}
	var args []string
	for _, arg := range os.Args[1:] {
		if strings.ContainsRune(arg, ' ') {
			args = append(args, strconv.Quote(arg))
		} else {
			args = append(args, arg)
		}
	}
	argsPtr, err := syscall.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, argsPtr, nil, syscall.SW_NORMAL)
}
