package main

import (
	"encoding/json"
	"os"
	"syscall"
	"time"
	"unsafe"
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
)

const (
	PROCESS_VM_READ = 0x0010
)

func main() {
	lastWindowCheckTime := time.Now()
	lastActiveWindowTitle, lastActiveAppName := getActiveWindowInfo()
	appInfoMap := make(map[string]*AppInfo)

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
	appInfoList := make([]AppInfo, 0, len(appInfoMap))
	for _, appInfo := range appInfoMap {
		appInfoList = append(appInfoList, *appInfo)
	}
	file, _ := json.MarshalIndent(appInfoList, "", " ")
	_ = os.WriteFile("activity.json", file, 0644)
}
