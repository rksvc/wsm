//go:build windows

package main

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

var Exe string

func init() {
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	Exe = exe
}

func main() {
	if isService, err := svc.IsWindowsService(); err != nil {
		panic(err)
	} else if isService {
		const WSM = "WSM"
		eventlog.InstallAsEventCreate(WSM, eventlog.Error|eventlog.Warning|eventlog.Info)
		elog, err := eventlog.Open(WSM)
		if err != nil {
			panic(err)
		}
		defer elog.Close()
		if err := svc.Run(WSM, &Handler{elog: elog}); err != nil {
			elog.Error(1, err.Error())
		}
		return
	}

	if !windows.GetCurrentProcessToken().IsElevated() {
		if err := elevate(); err != nil {
			panic(err)
		}
		return
	}

	m, err := NewModel()
	if err != nil {
		m.Error(err)
		os.Exit(1)
	}

	window := MainWindow{
		Title:    "Windows Service Manager",
		AssignTo: &m.mw,
		Layout:   VBox{Spacing: 3},
		Children: []Widget{
			Label{Text: "Services"},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					VSplitter{
						StretchFactor: 3,
						Children: []Widget{
							TableView{
								Name:     "tv",
								AssignTo: &m.tv,
								Model:    m.table,
								Columns: []TableViewColumn{
									{Title: "Name", DataMember: "Name"},
									{Title: "Startup type", DataMember: "StartType"},
									{Title: "Status", DataMember: "Status"},
									{Title: "Description", DataMember: "Description"},
								},
								StyleCell: func(style *walk.CellStyle) {
									if style.Col() == 2 {
										// https://astryx.atmeta.com/components/Badge?tab=properties
										switch svc.State(m.table.services[style.Row()].Status) {
										case svc.Stopped:
											style.TextColor = walk.RGB(137, 0, 26)
										case svc.StartPending, svc.ContinuePending:
											style.TextColor = walk.RGB(0, 69, 140)
										case svc.StopPending, svc.PausePending:
											style.TextColor = walk.RGB(110, 53, 0)
										case svc.Running:
											style.TextColor = walk.RGB(12, 87, 0)
										case svc.Paused:
											style.TextColor = walk.RGB(0, 83, 72)
										}
									}
								},
								OnCurrentIndexChanged: m.tableCurrentIndexChanged,
							},
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									PushButton{
										Text:      "New",
										Enabled:   Bind("tv.CurrentIndex != -1"),
										OnClicked: m.buttonNewServiceClick,
									},
									PushButton{
										Text:      Bind("action(tv.CurrentIndex)"),
										Enabled:   Bind("action_enabled(tv.CurrentIndex)"),
										OnClicked: m.actionClick,
									},
									PushButton{
										Text:      "Restart",
										Enabled:   Bind("restart_enabled(tv.CurrentIndex)"),
										OnClicked: m.buttonRestartClick,
									},
								},
								Functions: map[string]func(args ...any) (any, error){
									"action": func(args ...any) (any, error) {
										index := int(args[0].(float64))
										if index == -1 {
											return "Start", nil
										}
										switch svc.State(m.table.services[index].Status) {
										case svc.Stopped, svc.StopPending, svc.PausePending, svc.Paused:
											return "Start", nil
										case svc.StartPending, svc.Running, svc.ContinuePending:
											return "Stop", nil
										}
										return "", nil
									},
									"action_enabled": func(args ...any) (any, error) {
										index := int(args[0].(float64))
										if index == -1 {
											return false, nil
										}
										switch svc.State(m.table.services[index].Status) {
										case svc.Stopped, svc.Running, svc.Paused:
											return true, nil
										}
										return false, nil
									},
									"restart_enabled": func(args ...any) (any, error) {
										index := int(args[0].(float64))
										if index == -1 {
											return false, nil
										}
										switch svc.State(m.table.services[index].Status) {
										case svc.Running, svc.Paused:
											return true, nil
										}
										return false, nil
									},
								},
							},
						},
					},
					ScrollView{
						StretchFactor: 7,
						Layout:        VBox{Margins: Margins{Right: 20}},
						DataBinder:    DataBinder{AssignTo: &m.db, DataSource: &m.srv},
						Children: []Widget{
							GroupBox{
								Title:  "Config",
								Layout: Grid{Columns: 3},
								Children: []Widget{
									Label{Text: "Name (required)"},
									LineEdit{ColumnSpan: 2, Text: Bind("Name"), ReadOnly: Bind("tv.CurrentIndex != -1")},
									Label{Text: "Path (required)"},
									LineEdit{Text: Bind("Exe"), AssignTo: &m.exe},
									PushButton{Text: "Choose", OnClicked: m.chooseFileFor(&m.exe, false)},
									Label{Text: "Startup directory"},
									LineEdit{Text: Bind("Dir"), AssignTo: &m.dir},
									PushButton{Text: "Choose", OnClicked: m.chooseFileFor(&m.dir, true)},
									Label{Text: "Arguments"},
									LineEdit{ColumnSpan: 2, Text: Bind("Flags")},
									Label{Text: "Description"},
									LineEdit{ColumnSpan: 2, Text: Bind("Description")},
									Label{Text: "Input (stdin)"},
									LineEdit{Text: Bind("Stdin"), AssignTo: &m.stdin},
									PushButton{Text: "Choose", OnClicked: m.chooseFileFor(&m.stdin, false)},
									Label{Text: "Output (stdout)"},
									LineEdit{Text: Bind("Stdout"), AssignTo: &m.stdout},
									PushButton{Text: "Choose", OnClicked: m.chooseFileFor(&m.stdout, false)},
									Label{Text: "Error (stderr)"},
									LineEdit{Text: Bind("Stderr"), AssignTo: &m.stderr},
									PushButton{Text: "Choose", OnClicked: m.chooseFileFor(&m.stderr, false)},
									Label{Text: "Startup type"},
									ComboBox{
										ColumnSpan: 2,
										Value:      Bind("StartType"),
										Model:      []string{AUTO_START, AUTO_START_DELAYED, MANUAL, DISABLED},
									},
								},
							},
							GroupBox{
								Title:  "Environment variables",
								Layout: VBox{},
								Children: []Widget{TextEdit{
									Text:    Bind("Env"),
									VScroll: true,
									MinSize: Size{Height: 70},
								}},
							},
							GroupBox{
								Title:  "Dependencies",
								Layout: VBox{},
								Children: []Widget{TextEdit{
									Text:    Bind("Dependencies"),
									VScroll: true,
									MinSize: Size{Height: 70},
								}},
							},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 3},
								Children: []Widget{
									PushButton{
										Text:      "Remove",
										OnClicked: m.buttonRemoveClick,
										Enabled:   Bind("tv.CurrentIndex != -1"),
									},
									PushButton{
										Text:      "Save",
										OnClicked: m.buttonSaveClick,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if err := window.Create(); err != nil {
		m.Error(err)
		os.Exit(1)
	}

	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	hMon := win.MonitorFromWindow(m.mw.Handle(), win.MONITOR_DEFAULTTOPRIMARY)
	if win.GetMonitorInfo(hMon, &mi) {
		work := mi.RcWork
		const WIDTH, HEIGHT = 1300, 800
		err := m.mw.SetBoundsPixels(walk.Rectangle{
			X:      int(work.Left) + (int(work.Right-work.Left)-WIDTH)/2,
			Y:      int(work.Top) + (int(work.Bottom-work.Top)-HEIGHT)/2,
			Width:  WIDTH,
			Height: HEIGHT,
		})
		if err != nil {
			m.Error(NewError(err))
		}
	}

	m.mw.Run()
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
