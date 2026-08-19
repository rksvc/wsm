//go:build windows

package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type StartType int

const (
	AUTO_START StartType = iota
	AUTO_START_DELAYED
	MANUAL
	DISABLED
)

var StartTypeMap = map[StartType]uint32{
	AUTO_START:         windows.SERVICE_AUTO_START,
	AUTO_START_DELAYED: windows.SERVICE_AUTO_START,
	MANUAL:             windows.SERVICE_DEMAND_START,
	DISABLED:           windows.SERVICE_DISABLED,
}

var StateString = map[svc.State]string{
	svc.Stopped:         "Stopped",
	svc.StartPending:    "Starting",
	svc.StopPending:     "Stopping",
	svc.Running:         "Running",
	svc.ContinuePending: "Continuing",
	svc.PausePending:    "Pausing",
	svc.Paused:          "Paused",
}

type Server struct {
	m *mgr.Mgr
}

func New() (*Server, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	return &Server{m}, nil
}

type Service struct {
	Name        string    `json:"name"`
	StartType   StartType `json:"start_type"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header["Origin"]
		if len(origin) == 0 {
			return true
		}
		u, err := url.Parse(origin[0])
		if err != nil {
			return false
		}
		host := r.Host
		if i := strings.LastIndexByte(host, ':'); i != -1 {
			host = host[:i]
		}
		return strings.EqualFold(u.Hostname(), host)
	},
}

func (s *Server) Services(c *echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	var mu sync.Mutex
	services, err := s.m.ListServices()
	if err != nil {
		return ws.WriteJSON(NewServerError(err).Message)
	}
	srvs := make([]Service, 0)
	for _, name := range services {
		path := fmt.Sprintf("%s\\%s\\%s", REGISTRY, name, REG_PARAMETERS)
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
		if err != nil {
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			return ws.WriteJSON(NewServerError(err).Message)
		}

		_, _, err = key.GetStringValue(REG_EXE)
		key.Close()
		if err != nil {
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			return ws.WriteJSON(NewServerError(err).Message)
		}

		srv, err := s.m.OpenService(name)
		if err != nil {
			return ws.WriteJSON(NewServerError(err).Message)
		}
		defer srv.Close()
		callback := windows.NewCallback(func(notify uint32, context uintptr) uintptr {
			status, err := srv.Query()
			if err != nil {
				ws.WriteJSON(NewServerError(err).Message)
				return 0
			}
			go func() {
				mu.Lock()
				ws.WriteJSON(map[string]string{
					"name":   name,
					"status": StateString[status.State],
				})
				mu.Unlock()
			}()
			return 0
		})
		var subscription uintptr
		err = windows.SubscribeServiceChangeNotifications(srv.Handle, windows.SC_EVENT_STATUS_CHANGE, callback, 0, &subscription)
		if err != nil {
			return ws.WriteJSON(NewServerError(err).Message)
		}
		defer windows.UnsubscribeServiceChangeNotifications(subscription)

		c, err := srv.Config()
		if err != nil {
			return ws.WriteJSON(NewServerError(err).Message)
		}
		status, err := srv.Query()
		if err != nil {
			return ws.WriteJSON(NewServerError(err).Message)
		}
		var startType StartType
		switch c.StartType {
		case windows.SERVICE_AUTO_START:
			if c.DelayedAutoStart {
				startType = AUTO_START_DELAYED
			} else {
				startType = AUTO_START
			}
		case windows.SERVICE_DEMAND_START:
			startType = MANUAL
		case windows.SERVICE_DISABLED:
			startType = DISABLED
		}
		srvs = append(srvs, Service{
			Name:        name,
			StartType:   startType,
			Description: c.Description,
			Status:      StateString[status.State],
		})
	}
	mu.Lock()
	err = ws.WriteJSON(srvs)
	mu.Unlock()
	if err != nil {
		return err
	}

	for {
		if _, _, err := ws.NextReader(); err != nil {
			break
		}
	}
	return nil
}

type ServiceRequest struct {
	Dependencies []string  `json:"dependencies"`
	Description  string    `json:"description"`
	StartType    StartType `json:"start_type"`
	Config
}

func (s *Server) InstallService(c *echo.Context) error {
	var body struct {
		Name string `json:"name"`
		ServiceRequest
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	startType, ok := StartTypeMap[body.StartType]
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid start type.")
	}

	srv, err := s.m.CreateService(body.Name, Exe, mgr.Config{
		StartType:        startType,
		Dependencies:     body.Dependencies,
		Description:      body.Description,
		DelayedAutoStart: body.StartType == AUTO_START_DELAYED,
	})
	if err != nil {
		return NewServerError(err)
	}
	defer srv.Close()
	if err := SetConfig(body.Name, body.Config); err != nil {
		return NewServerError(err)
	}
	return c.JSON(http.StatusOK, struct{}{})
}

func (s *Server) EditService(c *echo.Context) error {
	var body ServiceRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	startType, ok := StartTypeMap[body.StartType]
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid start type.")
	}

	name := c.Param("name")
	srv, err := s.m.OpenService(name)
	if err != nil {
		return NewServerError(err)
	}
	defer srv.Close()
	err = srv.UpdateConfig(mgr.Config{
		StartType:        startType,
		Dependencies:     body.Dependencies,
		Description:      body.Description,
		DelayedAutoStart: body.StartType == AUTO_START_DELAYED,
	})
	if err != nil {
		return NewServerError(err)
	}
	if err := SetConfig(name, body.Config); err != nil {
		return NewServerError(err)
	}
	return c.JSON(http.StatusOK, struct{}{})
}

func (s *Server) ServiceProcesses(c *echo.Context) error {
	srv, err := s.m.OpenService(c.Param("name"))
	if err != nil {
		return NewServerError(err)
	}
	defer srv.Close()
	status, err := srv.Query()
	if err != nil {
		return NewServerError(err)
	}

	p, err := process.NewProcess(int32(status.ProcessId))
	if err != nil {
		return NewServerError(err)
	}
	type Tree struct {
		Pid      int32  `json:"pid"`
		Exe      string `json:"exe"`
		Children []Tree `json:"children,omitempty"`
	}
	var tree Tree
	type Task struct {
		Root    *Tree
		Process *process.Process
	}
	queue := []Task{{&tree, p}}
	for len(queue) > 0 {
		task := queue[0]
		queue = queue[1:]
		exe, err := task.Process.Exe()
		if err != nil {
			return NewServerError(err)
		}
		task.Root.Exe = exe
		task.Root.Pid = task.Process.Pid

		children, err := task.Process.Children()
		if err != nil {
			return NewServerError(err)
		}
		task.Root.Children = make([]Tree, len(children))
		for i, p := range children {
			queue = append(queue, Task{&task.Root.Children[i], p})
		}
	}
	return c.JSON(http.StatusOK, tree)
}

func (s *Server) StartService(c *echo.Context) error {
	srv, err := s.m.OpenService(c.Param("name"))
	if err != nil {
		return NewServerError(err)
	}
	defer srv.Close()
	err = srv.Start()
	if err != nil {
		return NewServerError(err)
	}
	return c.JSON(http.StatusOK, struct{}{})
}

func (s *Server) StopService(c *echo.Context) error {
	srv, err := s.m.OpenService(c.Param("name"))
	if err != nil {
		return NewServerError(err)
	}
	defer srv.Close()
	_, err = srv.Control(svc.Stop)
	if err != nil {
		return NewServerError(err)
	}
	return c.JSON(http.StatusOK, struct{}{})
}

func (s *Server) RestartService(c *echo.Context) error {
	srv, err := s.m.OpenService(c.Param("name"))
	if err != nil {
		return NewServerError(err)
	}
	defer srv.Close()
	status, err := srv.Control(svc.Stop)
	if err != nil {
		return NewServerError(err)
	}
	startTime := time.Now()
	for status.State != windows.SERVICE_STOPPED {
		if time.Since(startTime) > 30*time.Second {
			return NewServerError(errors.New("Service stop timed out."))
		}
		time.Sleep(time.Duration(status.WaitHint) * time.Millisecond)
		status, err = srv.Query()
		if err != nil {
			return NewServerError(err)
		}
	}
	err = srv.Start()
	if err != nil {
		return NewServerError(err)
	}
	return c.JSON(http.StatusOK, struct{}{})
}

func (s *Server) DeleteService(c *echo.Context) error {
	srv, err := s.m.OpenService(c.Param("name"))
	if err != nil {
		return NewServerError(err)
	}
	defer srv.Close()
	if err := srv.Delete(); err != nil {
		return NewServerError(err)
	}
	return c.JSON(http.StatusOK, struct{}{})
}

func NewServerError(err error) *echo.HTTPError {
	var s string
	_, file, line, ok := runtime.Caller(1)
	if ok {
		s = fmt.Sprintf("%s:%d: %s", path.Base(file), line, err)
	} else {
		s = err.Error()
	}
	return echo.NewHTTPError(http.StatusInternalServerError, s)
}
