//go:build windows

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

//go:embed icons/start.svg
var startSVG []byte

//go:embed icons/stop.svg
var stopSVG []byte

//go:embed icons/restart.svg
var restartSVG []byte

//go:embed icons/edit.svg
var editSVG []byte

var (
	start   = ui.MustParseSVG(startSVG)
	stop    = ui.MustParseSVG(stopSVG)
	restart = ui.MustParseSVG(restartSVG)
	edit    = ui.MustParseSVG(editSVG)
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
	Env string

	Name         string
	Dependencies string
	Description  string
	StartType    string
}

type Service struct {
	Name        string
	StartType   string
	Description string
	Status      Status

	srv          *mgr.Service
	subscription uintptr
}

type App struct {
	win  atomic.Value
	mgr  *mgr.Mgr
	srvs []*Service
	form ServiceConfig

	editing, new, asking bool
}

func NewApp() (*App, error) {
	mgr, err := mgr.Connect()
	if err != nil {
		return nil, NewError(err)
	}
	a := &App{mgr: mgr}

	srvs, err := mgr.ListServices()
	if err != nil {
		return nil, NewError(err)
	}
	for _, name := range srvs {
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
		callback := a.newServiceChangeNotificationCallback(name, srv)
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
		a.srvs = append(a.srvs, &Service{
			Name:        name,
			StartType:   getStartType(c.StartType, c.DelayedAutoStart),
			Status:      Status(status.State),
			Description: c.Description,

			srv:          srv,
			subscription: subscription,
		})
	}

	return a, nil
}

func (a *App) newServiceChangeNotificationCallback(name string, srv *mgr.Service) uintptr {
	return windows.NewCallback(func(notify uint32, context uintptr) uintptr {
		status, err := srv.Query()
		if err != nil {
			mygo.Dialog.Error("", NewError(err).Error())
			return 0
		}
		update := func() {
			index := slices.IndexFunc(a.srvs, func(s *Service) bool { return s.Name == name })
			if index != -1 {
				a.srvs[index].Status = Status(status.State)
			}
		}
		if win := a.win.Load(); win == nil {
			update()
		} else {
			win.(*mygo.Window).Update(update)
		}
		return 0
	})
}

func (a *App) view(c *ui.Context) {
	t := c.Theme()

	ui.Scroll(c).Children(func() {
		ui.Column(c).Padding(16).Gap(8).Children(func() {
			for _, srv := range a.srvs {
				ui.Row(c).Gap(8).Children(func() {
					ui.Column(c).Grow(1).Children(func() {
						ui.Row(c).Gap(4).Children(func() {
							ui.Text(c, srv.Name)

							var startType string
							switch srv.StartType {
							case AUTO_START:
								startType = "AutoStart"
							case AUTO_START_DELAYED:
								startType = "AutoStartDelayed"
							case MANUAL:
								startType = "Manual"
							case DISABLED:
								startType = "Disabled"
							}
							ui.Badge(c, startType)

							b := ui.Badge(c, srv.Status.String())
							// https://astryx.atmeta.com/components/Badge?tab=properties
							switch svc.State(srv.Status) {
							case svc.Stopped: // red
								if t.Dark {
									b.Background(ui.RGB(91, 43, 40)).TextColor(ui.RGB(255, 174, 167))
								} else {
									b.Background(ui.RGB(255, 196, 190)).TextColor(ui.RGB(118, 0, 12))
								}
							case svc.StartPending, svc.ContinuePending: // blue
								if t.Dark {
									b.Background(ui.RGB(37, 60, 90)).TextColor(ui.RGB(161, 202, 255))
								} else {
									b.Background(ui.RGB(185, 215, 255)).TextColor(ui.RGB(0, 57, 120))
								}
							case svc.StopPending, svc.PausePending: // orange
								if t.Dark {
									b.Background(ui.RGB(80, 52, 36)).TextColor(ui.RGB(255, 179, 126))
								} else {
									b.Background(ui.RGB(255, 199, 161)).TextColor(ui.RGB(98, 39, 0))
								}
							case svc.Running: // green
								if t.Dark {
									b.Background(ui.RGB(43, 66, 43)).TextColor(ui.RGB(144, 202, 144))
								} else {
									b.Background(ui.RGB(188, 224, 187)).TextColor(ui.RGB(0, 73, 11))
								}
							case svc.Paused:
								b.TextColor(t.TextMuted)
							}
						})
						if srv.Description != "" {
							ui.Text(c, srv.Description).FontSize(12).TextColor(t.TextMuted)
						}
					})

					running := false
					tooltip := "Start"
					switch svc.State(srv.Status) {
					case svc.StartPending, svc.Running, svc.ContinuePending:
						running = true
						tooltip = "Stop"
					}
					disabled := true
					switch svc.State(srv.Status) {
					case svc.Stopped, svc.Running, svc.Paused:
						disabled = false
					}
					if ui.Button(c, "").Tooltip(tooltip).Disabled(disabled).Children(func() {
						if running {
							ui.Icon(c, stop)
						} else {
							ui.Icon(c, start)
						}
					}).Clicked() {
						var err error
						if srv.Status == Status(svc.Running) {
							_, err = srv.srv.Control(svc.Stop)
						} else {
							err = srv.srv.Start()
						}
						if err != nil {
							mygo.Dialog.Error("", NewError(err).Error())
						}
					}

					disabled = true
					switch svc.State(srv.Status) {
					case svc.Running, svc.Paused:
						disabled = false
					}
					if ui.Button(c, "").Tooltip("Restart").Disabled(disabled).Children(func() {
						ui.Icon(c, restart)
					}).Clicked() {
						go func() {
							status, err := srv.srv.Control(svc.Stop)
							if err != nil {
								mygo.Dialog.Error("", NewError(err).Error())
								return
							}
							startTime := time.Now()
							for status.State != windows.SERVICE_STOPPED {
								if time.Since(startTime) > 30*time.Second {
									mygo.Dialog.Error("", "Service stop timed out.")
									return
								}
								time.Sleep(time.Duration(status.WaitHint) * time.Millisecond)
								status, err = srv.srv.Query()
								if err != nil {
									mygo.Dialog.Error("", NewError(err).Error())
									return
								}
							}
							if err = srv.srv.Start(); err != nil {
								mygo.Dialog.Error("", NewError(err).Error())
							}
						}()
					}

					if ui.Button(c, "").Tooltip("Edit").Children(func() {
						ui.Icon(c, edit)
					}).Clicked() {
						if cfg, err := srv.srv.Config(); err != nil {
							mygo.Dialog.Error("", NewError(err).Error())
						} else if config, err := GetConfig(srv.Name); err != nil {
							mygo.Dialog.Error("", NewError(err).Error())
						} else {
							a.form = ServiceConfig{
								Config: *config,
								Env:    strings.Join(config.Env, "\n"),

								Name:         srv.Name,
								Dependencies: strings.Join(cfg.Dependencies, "\n"),
								Description:  cfg.Description,
								StartType:    getStartType(cfg.StartType, cfg.DelayedAutoStart),
							}

							a.editing = true
							a.new = false
						}
					}
				})
			}
		})
	})

	t.Spacing = 3
	ui.Modal(c, &a.editing, func() {
		ui.Scroll(c).PaddingEnd(20).Children(func() {
			ui.Form(c, func() {
				ui.Field(c, "Name", func() { ui.TextInput(c, &a.form.Name).ReadOnly(!a.new) })
				ui.Field(c, "Path", func() {
					ui.Row(c).Gap(6).Children(func() {
						ui.TextInput(c, &a.form.Exe).Grow(1)
						a.selectFileButton(c, &a.form.Exe, false)
					})
				})
				ui.Field(c, "Startup directory", func() {
					ui.Row(c).Gap(6).Children(func() {
						ui.TextInput(c, &a.form.Dir).Grow(1)
						a.selectFileButton(c, &a.form.Dir, true)
					})
				})
				ui.Field(c, "Arguments", func() { ui.TextInput(c, &a.form.Flags) })
				ui.Field(c, "Description", func() { ui.TextInput(c, &a.form.Description) })
				ui.Field(c, "Input (stdin)", func() {
					ui.Row(c).Gap(6).Children(func() {
						ui.TextInput(c, &a.form.Stdin).Grow(1)
						a.selectFileButton(c, &a.form.Stdin, false)
					})
				})
				ui.Field(c, "Output (stdout)", func() {
					ui.Row(c).Gap(6).Children(func() {
						ui.TextInput(c, &a.form.Stdout).Grow(1)
						a.selectFileButton(c, &a.form.Stdout, false)
					})
				})
				ui.Field(c, "Error (stderr)", func() {
					ui.Row(c).Gap(6).Children(func() {
						ui.TextInput(c, &a.form.Stderr).Grow(1)
						a.selectFileButton(c, &a.form.Stderr, false)
					})
				})
				ui.Field(c, "Startup type", func() {
					ui.Select(c, &a.form.StartType, []string{AUTO_START, AUTO_START_DELAYED, MANUAL, DISABLED})
				})
				ui.Field(c, "Environment variables", func() {
					ui.TextArea(c, &a.form.Env)
				})
				ui.Field(c, "Dependencies", func() {
					ui.TextArea(c, &a.form.Dependencies)
				})
			})
		})

		ui.Row(c).PaddingEnd(20).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "Cancel").Clicked() {
				a.editing = false
			}
			if !a.new && ui.Button(c, "Delete").Background(t.Danger).TextColor(ui.RGB(255, 255, 255)).Clicked() {
				a.asking = true
			}
			if ui.PrimaryButton(c, "Save").Clicked() {
				if a.form.Name == "" {
					mygo.Dialog.Error("", "Name is required.")
				} else if a.form.Exe == "" {
					mygo.Dialog.Error("", "Path is required.")
				} else {
					a.editing = !a.save()
				}
			}
		})
	}).FontSize(13)

	if ui.AlertDialog(c, &a.asking, "Delete "+a.form.Name+"?", "You can't undo this.", "Cancel", "Delete") == 1 {
		i := slices.IndexFunc(a.srvs, func(s *Service) bool { return s.Name == a.form.Name })
		srv := a.srvs[i]
		err := windows.UnsubscribeServiceChangeNotifications(srv.subscription)
		if err != nil {
			mygo.Dialog.Error("", NewError(err).Error())
		} else if err := srv.srv.Delete(); err != nil {
			mygo.Dialog.Error("", NewError(err).Error())
		} else {
			srv.srv.Close()
			a.srvs = slices.Delete(a.srvs, i, i+1)
			a.editing = false
		}
	}
}

func (a *App) save() bool {
	startType, ok := StartTypeMap[a.form.StartType]
	if !ok {
		panic("invalid startup type: " + a.form.StartType)
	}
	a.form.Config.Env = SplitLines(a.form.Env)

	if a.new {
		delayed := a.form.StartType == AUTO_START_DELAYED
		srv, err := a.mgr.CreateService(a.form.Name, Exe, mgr.Config{
			StartType:        startType,
			Dependencies:     SplitLines(a.form.Dependencies),
			Description:      a.form.Description,
			DelayedAutoStart: delayed,
		})
		if err != nil {
			mygo.Dialog.Error("", NewError(err).Error())
			return false
		}
		if err := SetConfig(a.form.Name, a.form.Config); err != nil {
			srv.Delete()
			mygo.Dialog.Error("", NewError(err).Error())
			return false
		}

		callback := a.newServiceChangeNotificationCallback(a.form.Name, srv)
		var subscription uintptr
		err = windows.SubscribeServiceChangeNotifications(srv.Handle, windows.SC_EVENT_STATUS_CHANGE, callback, 0, &subscription)
		if err != nil {
			srv.Delete()
			mygo.Dialog.Error("", NewError(err).Error())
			return false
		}

		a.srvs = append(a.srvs, &Service{
			Name:        a.form.Name,
			StartType:   getStartType(startType, delayed),
			Description: a.form.Description,

			srv:          srv,
			subscription: subscription,
		})
	} else {
		i := slices.IndexFunc(a.srvs, func(s *Service) bool { return s.Name == a.form.Name })
		srv := a.srvs[i].srv
		deps := SplitLines(a.form.Dependencies)
		delayed := a.form.StartType == AUTO_START_DELAYED
		err := srv.UpdateConfig(mgr.Config{
			ServiceType:      windows.SERVICE_NO_CHANGE,
			ErrorControl:     windows.SERVICE_NO_CHANGE,
			StartType:        startType,
			Dependencies:     deps,
			Description:      a.form.Description,
			DelayedAutoStart: delayed,
		})
		if err != nil {
			mygo.Dialog.Error("", NewError(err).Error())
			return false
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
				mygo.Dialog.Error("", NewError(err).Error())
				return false
			}
		}

		if err := SetConfig(a.form.Name, a.form.Config); err != nil {
			mygo.Dialog.Error("", NewError(err).Error())
			return false
		}

		a.srvs[i].StartType = getStartType(startType, delayed)
		a.srvs[i].Description = a.form.Description
	}
	return true
}

func (a *App) selectFileButton(c *ui.Context, value *string, dir bool) {
	if ui.Button(c, "...").Clicked() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win.Load().(*mygo.Window),
			Directory: dir,
		})
		if err != nil {
			mygo.Dialog.Error("", NewError(err).Error())
			return
		}
		if len(paths) > 0 {
			*value = paths[0]
		}
	}
}

func getStartType(startType uint32, delayedAutoStart bool) string {
	switch startType {
	case windows.SERVICE_AUTO_START:
		if delayedAutoStart {
			return AUTO_START_DELAYED
		}
		return AUTO_START
	case windows.SERVICE_DEMAND_START:
		return MANUAL
	case windows.SERVICE_DISABLED:
		return DISABLED
	}
	panic(fmt.Errorf("invalid start type: %d", startType))
}
