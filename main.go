package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/getlantern/systray"
)

type TabInfo struct {
	Title    string        `json:"title"`
	Duration time.Duration `json:"duration"`
}

type AppInfo struct {
	AppName string    `json:"app_name"`
	Tabs    []TabInfo `json:"tabs"`
}

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	psapi                        = syscall.NewLazyDLL("psapi.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetModuleBaseNameW       = psapi.NewProc("GetModuleBaseNameW")
	appInfoMap                   map[string]*AppInfo
	appInfoLock                  sync.Mutex // Mutex for synchronizing access to appInfoMap
	showUIChan                   = make(chan bool, 1)
	fyneApp                      fyne.App
	w                            fyne.Window
)

const (
	PROCESS_VM_READ = 0x0010
)

func main() {
	appInfoMap = make(map[string]*AppInfo) // Initialize appInfoMap here
	go systray.Run(onReady, onExit)        // Run systray in a new goroutine

	// Start a new goroutine for your existing window tracking logic
	go func() {
		lastWindowCheckTime := time.Now()
		lastActiveWindowTitle, lastActiveAppName := getActiveWindowInfo()

		for {
			currentActiveWindowTitle, currentActiveAppName := getActiveWindowInfo()
			if currentActiveAppName != lastActiveAppName || currentActiveWindowTitle != lastActiveWindowTitle {
				duration := time.Since(lastWindowCheckTime)
				appInfo, exists := appInfoMap[lastActiveAppName]
				if !exists {
					appInfo = &AppInfo{AppName: lastActiveAppName}
					appInfoMap[lastActiveAppName] = appInfo
				}
				tabInfo := TabInfo{Title: lastActiveWindowTitle, Duration: duration}
				appInfo.Tabs = append(appInfo.Tabs, tabInfo)
				saveActivity(appInfoMap)
				lastWindowCheckTime = time.Now()
				lastActiveWindowTitle = currentActiveWindowTitle
				lastActiveAppName = currentActiveAppName
			}
			time.Sleep(100 * time.Millisecond) // short sleep to prevent busy-waiting
		}
	}()

	// Main loop to listen for showUI signal
	for {
		select {
		case <-showUIChan:
			showUI()
		default:
			// Optional: add a short sleep here to prevent busy-waiting
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func getActiveWindowInfo() (string, string) {
	hwnd, _, _ := procGetForegroundWindow.Call()

	// Get window title
	buff := make([]uint16, 512) // adjust buffer size if necessary
	buffSize := uintptr(512)
	textLen, _, _ := procGetWindowTextW.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&buff[0])),
		buffSize,
	)
	title := syscall.UTF16ToString(buff[:textLen])

	// Get application name
	var processID uint32
	procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&processID)))
	hProcess, _ := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION|PROCESS_VM_READ, false, processID)
	defer syscall.CloseHandle(hProcess)
	buff = make([]uint16, 512) // adjust buffer size if necessary
	appLen, _, _ := procGetModuleBaseNameW.Call(
		uintptr(hProcess),
		0,
		uintptr(unsafe.Pointer(&buff[0])),
		buffSize,
	)
	appName := syscall.UTF16ToString(buff[:appLen])

	return title, appName
}

func saveActivity(appInfoMap map[string]*AppInfo) {
	appInfoLock.Lock() // Lock before accessing appInfoMap
	defer appInfoLock.Unlock()
	appInfoList := make([]AppInfo, 0, len(appInfoMap))
	for _, appInfo := range appInfoMap {
		appInfoList = append(appInfoList, *appInfo)
	}
	file, _ := json.MarshalIndent(appInfoList, "", " ")
	_ = os.WriteFile("activity.json", file, 0644)
}

func onReady() {
	systray.SetIcon(getIcon()) // Replace with your icon
	systray.SetTitle("Time Tracker")
	systray.SetTooltip("Click to view time spent")
	mShow := systray.AddMenuItem("Show", "Show Time Tracker")
	mQuit := systray.AddMenuItem("Quit", "Quit Time Tracker")
	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			case <-mShow.ClickedCh:
				showUIChan <- true
			}
		}
	}()
}

func onExit() {
	// Save the latest activity data to disk
	saveActivity(appInfoMap)
}

func showUI() {
	if fyneApp == nil {
		fyneApp = app.New() // create a new app
		w = fyneApp.NewWindow("Time Tracker")
		setupWindowContent(w) // move the window setup code to a new function
	} else {
		w.Show() // show the existing window
	}
}

func setupWindowContent(w fyne.Window) {
	list := widget.NewList(
		func() int {
			appInfoLock.Lock()
			defer appInfoLock.Unlock()
			return len(appInfoMap)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			label := o.(*widget.Label)
			appInfoLock.Lock()
			defer appInfoLock.Unlock()
			var index int
			var appName string
			var total time.Duration
			for currentAppName, appInfo := range appInfoMap {
				if index == int(i) {
					appName = currentAppName
					total = getTotalDuration(appInfo)
					break
				}
				index++
			}
			label.SetText(fmt.Sprintf("%s: %v", appName, total))
		},
	)
	w.SetContent(container.NewScroll(list))
	w.ShowAndRun()
}

func getTotalDuration(appInfo *AppInfo) time.Duration {
	var total time.Duration
	for _, tab := range appInfo.Tabs {
		total += tab.Duration
	}
	return total
}

func getIcon() []byte {
	data, err := os.ReadFile("clock.ico")
	if err != nil {
		fmt.Println("Error reading icon file:", err)
		return nil
	}
	return data
}
