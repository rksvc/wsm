//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/kballard/go-shellquote"
	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type Status svc.State

var stateString = map[svc.State]string{
	svc.Stopped:         "Stopped",
	svc.StartPending:    "Starting",
	svc.StopPending:     "Stopping",
	svc.Running:         "Running",
	svc.ContinuePending: "Continuing",
	svc.PausePending:    "Pausing",
	svc.Paused:          "Paused",
}

func (s Status) String() string {
	return stateString[svc.State(s)]
}

const (
	AUTO_START         = "Automatic"
	AUTO_START_DELAYED = "Automatic (Delayed Start)"
	MANUAL             = "Manual"
	DISABLED           = "Disabled"
)

var StartTypeMap = map[string]uint32{
	AUTO_START:         windows.SERVICE_AUTO_START,
	AUTO_START_DELAYED: windows.SERVICE_AUTO_START,
	MANUAL:             windows.SERVICE_DEMAND_START,
	DISABLED:           windows.SERVICE_DISABLED,
}

type ServiceConfig struct {
	Config
	Flags string
	Env   string

	Name         string
	Dependencies string
	Description  string
	StartType    string
}

func NewServiceConfig() ServiceConfig {
	return ServiceConfig{StartType: AUTO_START}
}

type Service struct {
	Name        string
	StartType   string
	Description string
	Status      Status

	srv          *mgr.Service
	subscription uintptr
}

type TableModel struct {
	walk.SortedReflectTableModelBase
	services []*Service
}

func (m *TableModel) Items() any {
	return m.services
}

type Model struct {
	mgr *mgr.Mgr
	srv ServiceConfig

	table *TableModel

	mw *walk.MainWindow
	tv *walk.TableView
	db *walk.DataBinder
	cb *walk.ComboBox

	exe, dir, stdin, stdout, stderr *walk.LineEdit
}

func NewModel() (*Model, error) {
	mgr, err := mgr.Connect()
	if err != nil {
		return nil, NewError(err)
	}
	m := &Model{
		mgr:   mgr,
		srv:   NewServiceConfig(),
		table: &TableModel{},
	}

	services, err := mgr.ListServices()
	if err != nil {
		return nil, NewError(err)
	}
	for _, name := range services {
		path := fmt.Sprintf("%s\\%s\\%s", REGISTRY, name, REG_PARAMETERS)
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
		if err != nil {
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			return nil, NewError(err)
		}

		_, _, err = key.GetStringValue(REG_EXE)
		key.Close()
		if err != nil {
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			return nil, NewError(err)
		}

		srv, err := mgr.OpenService(name)
		if err != nil {
			return nil, NewError(err)
		}
		callback := m.newServiceChangeNotificationCallback(name, srv)
		var subscription uintptr
		err = windows.SubscribeServiceChangeNotifications(srv.Handle, windows.SC_EVENT_STATUS_CHANGE, callback, 0, &subscription)
		if err != nil {
			return nil, NewError(err)
		}

		c, err := srv.Config()
		if err != nil {
			return nil, NewError(err)
		}
		status, err := srv.Query()
		if err != nil {
			return nil, NewError(err)
		}
		m.table.services = append(m.table.services, &Service{
			Name:        name,
			StartType:   getStartType(c.StartType, c.DelayedAutoStart),
			Status:      Status(status.State),
			Description: c.Description,

			srv:          srv,
			subscription: subscription,
		})
	}

	return m, nil
}

func (m *Model) newServiceChangeNotificationCallback(name string, srv *mgr.Service) uintptr {
	return windows.NewCallback(func(notify uint32, context uintptr) uintptr {
		status, err := srv.Query()
		if err != nil {
			m.Error(err)
			return 0
		}
		index := slices.IndexFunc(m.table.services, func(s *Service) bool {
			return s.Name == name
		})
		if index == -1 {
			return 0
		}

		m.table.services[index].Status = Status(status.State)
		m.table.PublishRowChanged(index)
		if m.tv != nil {
			if err := m.tv.SetCurrentIndex(m.tv.CurrentIndex()); err != nil {
				m.Error(err)
			}
		}
		return 0
	})
}

func (m *Model) tableCurrentIndexChanged() {
	index := m.tv.CurrentIndex()
	if index == -1 {
		m.srv = NewServiceConfig()
	} else {
		srv := m.table.services[index]
		cfg, err := srv.srv.Config()
		if err != nil {
			m.Error(NewError(err))
			return
		}
		config, err := GetConfig(srv.Name)
		if err != nil {
			m.Error(NewError(err))
			return
		}
		m.srv = ServiceConfig{
			Config: *config,
			Flags:  shellquote.Join(config.Flags...),
			Env:    strings.Join(config.Env, "\r\n"),

			Name:         srv.Name,
			Dependencies: strings.Join(cfg.Dependencies, "\r\n"),
			Description:  cfg.Description,
			StartType:    getStartType(cfg.StartType, cfg.DelayedAutoStart),
		}
	}
	if err := m.db.Reset(); err != nil {
		m.Error(err)
	}
}

func (m *Model) buttonNewServiceClick() {
	if err := m.tv.SetCurrentIndex(-1); err != nil {
		m.Error(err)
	}
}

func (m *Model) actionClick() {
	srv := m.table.services[m.tv.CurrentIndex()]
	var err error
	if srv.Status == Status(svc.Running) {
		_, err = srv.srv.Control(svc.Stop)
	} else {
		err = srv.srv.Start()
	}
	if err != nil {
		m.Error(NewError(err))
	}
}

func (m *Model) buttonRestartClick() {
	srv := m.table.services[m.tv.CurrentIndex()].srv
	status, err := srv.Control(svc.Stop)
	if err != nil {
		m.Error(NewError(err))
		return
	}
	startTime := time.Now()
	for status.State != windows.SERVICE_STOPPED {
		if time.Since(startTime) > 30*time.Second {
			m.Error(NewError(errors.New("Service stop timed out.")))
			return
		}
		time.Sleep(time.Duration(status.WaitHint) * time.Millisecond)
		status, err = srv.Query()
		if err != nil {
			m.Error(NewError(err))
			return
		}
	}
	if err = srv.Start(); err != nil {
		m.Error(NewError(err))
	}
}

func (m *Model) buttonRemoveClick() {
	index := m.tv.CurrentIndex()
	srv := m.table.services[index]
	if walk.MsgBox(m.mw, "", "Are you sure to delete "+srv.Name+"?", walk.MsgBoxYesNo) == walk.DlgCmdYes {
		err := windows.UnsubscribeServiceChangeNotifications(srv.subscription)
		if err != nil {
			m.Error(err)
			return
		} else if err := srv.srv.Delete(); err != nil {
			m.Error(err)
			return
		}
		srv.srv.Close()
		m.table.services = slices.Delete(m.table.services, index, index+1)
		m.table.PublishRowsRemoved(index, index)
	}
}

func (m *Model) buttonSaveClick() {
	err := m.db.Submit()
	if err != nil {
		m.Error(err)
		return
	}

	if m.srv.Name == "" {
		m.Error(errors.New("Name is required."))
		return
	} else if m.srv.Exe == "" {
		m.Error(errors.New("Path is required."))
		return
	}

	startType, ok := StartTypeMap[m.srv.StartType]
	if !ok {
		m.Error(errors.New("Invalid startup type."))
		return
	}
	m.srv.Config.Flags, err = shellquote.Split(m.srv.Flags)
	if err != nil {
		m.Error(NewError(err))
		return
	}
	m.srv.Config.Env = SplitNewline(m.srv.Env)

	index := m.tv.CurrentIndex()
	if index == -1 {
		delayed := m.srv.StartType == AUTO_START_DELAYED
		srv, err := m.mgr.CreateService(m.srv.Name, Exe, mgr.Config{
			StartType:        startType,
			Dependencies:     SplitNewline(m.srv.Dependencies),
			Description:      m.srv.Description,
			DelayedAutoStart: delayed,
		})
		if err != nil {
			m.Error(NewError(err))
			return
		}
		if err := SetConfig(m.srv.Name, m.srv.Config); err != nil {
			srv.Delete()
			m.Error(NewError(err))
			return
		}

		callback := m.newServiceChangeNotificationCallback(m.srv.Name, srv)
		var subscription uintptr
		err = windows.SubscribeServiceChangeNotifications(srv.Handle, windows.SC_EVENT_STATUS_CHANGE, callback, 0, &subscription)
		if err != nil {
			srv.Delete()
			m.Error(NewError(err))
			return
		}

		m.table.services = append(m.table.services, &Service{
			Name:        m.srv.Name,
			StartType:   getStartType(startType, delayed),
			Description: m.srv.Description,

			srv:          srv,
			subscription: subscription,
		})
		index = len(m.table.services) - 1
		m.table.PublishRowsInserted(index, index)
		if err := m.tv.SetCurrentIndex(-1); err != nil {
			m.Error(err)
			return
		}
	} else {
		srv := m.table.services[index].srv
		deps := SplitNewline(m.srv.Dependencies)
		delayed := m.srv.StartType == AUTO_START_DELAYED
		err = srv.UpdateConfig(mgr.Config{
			ServiceType:      windows.SERVICE_NO_CHANGE,
			ErrorControl:     windows.SERVICE_NO_CHANGE,
			StartType:        startType,
			Dependencies:     deps,
			Description:      m.srv.Description,
			DelayedAutoStart: delayed,
		})
		if err != nil {
			m.Error(NewError(err))
			return
		}
		if len(deps) == 0 {
			deps := []uint16{0}
			err := windows.ChangeServiceConfig(
				srv.Handle,
				windows.SERVICE_NO_CHANGE,
				windows.SERVICE_NO_CHANGE,
				windows.SERVICE_NO_CHANGE,
				nil,
				nil,
				nil,
				&deps[0],
				nil,
				nil,
				nil,
			)
			if err != nil {
				m.Error(NewError(err))
				return
			}
		}

		if err := SetConfig(m.srv.Name, m.srv.Config); err != nil {
			m.Error(NewError(err))
			return
		}

		m.table.services[index].StartType = getStartType(startType, delayed)
		m.table.services[index].Description = m.srv.Description
		m.table.PublishRowChanged(index)
	}
	m.Info("Done!")
}

func (m *Model) chooseFileFor(edit **walk.LineEdit, folder bool) func() {
	return func() {
		var owner walk.Form
		if m.mw != nil {
			owner = m.mw
		}
		var dlg walk.FileDialog
		var accept bool
		var err error
		if folder {
			accept, err = dlg.ShowBrowseFolder(owner)
		} else {
			accept, err = dlg.ShowOpen(owner)
		}
		if err != nil {
			m.Error(err)
		} else if accept {
			if err := (*edit).SetText(dlg.FilePath); err != nil {
				m.Error(err)
			}
		}
	}
}

func (m *Model) Info(msg string) {
	var owner walk.Form
	if m.mw != nil {
		owner = m.mw
	}
	walk.MsgBox(owner, "", msg, walk.MsgBoxIconInformation)
}

func (m *Model) Error(err error) {
	var owner walk.Form
	if m != nil && m.mw != nil {
		owner = m.mw
	}
	walk.MsgBox(owner, "", err.Error(), walk.MsgBoxIconError)
}

func (m *Model) Fatal(err error) {
	m.Error(err)
	os.Exit(1)
}

func getStartType(startType uint32, delayedAutoStart bool) string {
	switch startType {
	case windows.SERVICE_AUTO_START:
		if delayedAutoStart {
			return AUTO_START_DELAYED
		} else {
			return AUTO_START
		}
	case windows.SERVICE_DEMAND_START:
		return MANUAL
	case windows.SERVICE_DISABLED:
		return DISABLED
	}
	panic(fmt.Errorf("invalid start type: %d", startType))
}
