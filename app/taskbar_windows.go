//go:build windows

package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Own taskbar identity for the app window.
//
// The window is an Edge/Chrome window in app mode. Left alone, Windows groups
// it with "Microsoft Edge": pinning it pins Edge, with Edge's icon.
//
// Here EngTreeViewer.exe finds that window and sets the Windows taskbar
// properties (AppUserModel) on it: its own ID, the relaunch command (this
// .exe), the display name and the app icon. The taskbar button is then
// separate from Edge and a pinned shortcut opens EngTree Viewer.

const (
	appUserModelID = "EngTree.Viewer"
	appDisplayName = "EngTree Viewer"
	windowTitle    = "EngTree Viewer" // must match <title> in ui/index.html
)

var (
	user32  = syscall.NewLazyDLL("user32.dll")
	shell32 = syscall.NewLazyDLL("shell32.dll")
	ole32   = syscall.NewLazyDLL("ole32.dll")
	kernel  = syscall.NewLazyDLL("kernel32.dll")

	procEnumWindows                 = user32.NewProc("EnumWindows")
	procGetWindowTextW              = user32.NewProc("GetWindowTextW")
	procGetClassNameW               = user32.NewProc("GetClassNameW")
	procIsWindowVisible             = user32.NewProc("IsWindowVisible")
	procSendMessageW                = user32.NewProc("SendMessageW")
	procLoadImageW                  = user32.NewProc("LoadImageW")
	procGetModuleHandleW            = kernel.NewProc("GetModuleHandleW")
	procSHGetPropertyStoreForWindow = shell32.NewProc("SHGetPropertyStoreForWindow")
	procCoInitializeEx              = ole32.NewProc("CoInitializeEx")
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type propertyKey struct {
	Fmtid guid
	Pid   uint32
}

// PROPVARIANT holding a VT_LPWSTR (24 bytes on 64-bit)
type propVariant struct {
	Vt       uint16
	reserved [3]uint16
	Val      uintptr
	pad      uintptr
}

var (
	iidIPropertyStore = guid{0x886d8eeb, 0x8cf2, 0x4446, [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}
	appUserModelFmtid = guid{0x9F4C2855, 0x9F79, 0x4B39, [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}}

	pkeyID                   = propertyKey{appUserModelFmtid, 5}
	pkeyRelaunchCommand      = propertyKey{appUserModelFmtid, 2}
	pkeyRelaunchIconResource = propertyKey{appUserModelFmtid, 3}
	pkeyRelaunchDisplayName  = propertyKey{appUserModelFmtid, 4}
)

const (
	// Method slots in the IPropertyStore vtable
	pkeyReleaseIndex  = 2
	pkeyGetValueIndex = 5
	pkeySetValueIndex = 6
	pkeyCommitIndex   = 7

	vtLPWSTR      uint16 = 31
	wmSetIcon            = 0x0080
	imageIcon            = 1
	lrDefaultSize        = 0x0040
	lrShared             = 0x8000
)

// startTaskbarIdentity watches for the app window and applies (and
// re-applies, if the browser resets it) the taskbar identity.
func startTaskbarIdentity() {
	go func() {
		runtime.LockOSThread()
		procCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED

		exe, err := os.Executable()
		if err != nil {
			return
		}
		exe, _ = filepath.Abs(exe)
		relaunch := `"` + exe + `"`
		icon := exe + ",0"

		bigIcon, smallIcon := loadAppIcons()

		done := map[uintptr]bool{}
		for {
			for _, hwnd := range findAppWindows() {
				if done[hwnd] && currentAppID(hwnd) == appUserModelID {
					continue
				}
				if applyIdentity(hwnd, relaunch, icon) {
					done[hwnd] = true
				}
				if bigIcon != 0 {
					procSendMessageW.Call(hwnd, wmSetIcon, 1, bigIcon)
				}
				if smallIcon != 0 {
					procSendMessageW.Call(hwnd, wmSetIcon, 0, smallIcon)
				}
			}
			time.Sleep(2 * time.Second)
		}
	}()
}

func loadAppIcons() (big, small uintptr) {
	hmod, _, _ := procGetModuleHandleW.Call(0)
	// Icon 1 = icon group embedded by rsrc_windows_amd64.syso
	big, _, _ = procLoadImageW.Call(hmod, 1, imageIcon, 256, 256, lrShared)
	if big == 0 {
		big, _, _ = procLoadImageW.Call(hmod, 1, imageIcon, 0, 0, lrDefaultSize|lrShared)
	}
	small, _, _ = procLoadImageW.Call(hmod, 1, imageIcon, 32, 32, lrShared)
	return
}

func findAppWindows() []uintptr {
	var found []uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		cls := make([]uint16, 128)
		procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&cls[0])), uintptr(len(cls)))
		if !strings.HasPrefix(syscall.UTF16ToString(cls), "Chrome_WidgetWin_") {
			return 1
		}
		title := make([]uint16, 256)
		procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		if strings.HasPrefix(syscall.UTF16ToString(title), windowTitle) {
			found = append(found, hwnd)
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return found
}

func propertyStore(hwnd uintptr) (uintptr, []uintptr) {
	var store uintptr
	hr, _, _ := procSHGetPropertyStoreForWindow.Call(hwnd, uintptr(unsafe.Pointer(&iidIPropertyStore)), uintptr(unsafe.Pointer(&store)))
	if int32(hr) < 0 || store == 0 {
		return 0, nil
	}
	vtbl := *(*uintptr)(unsafe.Pointer(store))
	methods := unsafe.Slice((*uintptr)(unsafe.Pointer(vtbl)), 8)
	return store, methods
}

func currentAppID(hwnd uintptr) string {
	store, m := propertyStore(hwnd)
	if store == 0 {
		return ""
	}
	defer syscall.SyscallN(m[pkeyReleaseIndex], store)
	var pv propVariant
	hr, _, _ := syscall.SyscallN(m[pkeyGetValueIndex], store, uintptr(unsafe.Pointer(&pkeyID)), uintptr(unsafe.Pointer(&pv)))
	if int32(hr) < 0 || pv.Vt != vtLPWSTR || pv.Val == 0 {
		return ""
	}
	// Read up to the terminating zero (256 chars max). The value is
	// allocated by the system and not freed here: a few bytes leaked, only
	// during the periodic check.
	var buf []uint16
	for i := uintptr(0); i < 256; i++ {
		ch := *(*uint16)(unsafe.Pointer(pv.Val + i*2))
		if ch == 0 {
			break
		}
		buf = append(buf, ch)
	}
	return syscall.UTF16ToString(buf)
}

func applyIdentity(hwnd uintptr, relaunch, icon string) bool {
	store, m := propertyStore(hwnd)
	if store == 0 {
		return false
	}
	defer syscall.SyscallN(m[pkeyReleaseIndex], store)

	set := func(key *propertyKey, value string) bool {
		s, err := syscall.UTF16PtrFromString(value)
		if err != nil {
			return false
		}
		pv := propVariant{Vt: vtLPWSTR, Val: uintptr(unsafe.Pointer(s))}
		hr, _, _ := syscall.SyscallN(m[pkeySetValueIndex], store, uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&pv)))
		runtime.KeepAlive(s)
		return int32(hr) >= 0
	}
	// Relaunch properties must be set before the ID, as Windows requires.
	ok := set(&pkeyRelaunchCommand, relaunch)
	ok = set(&pkeyRelaunchDisplayName, appDisplayName) && ok
	ok = set(&pkeyRelaunchIconResource, icon) && ok
	ok = set(&pkeyID, appUserModelID) && ok
	syscall.SyscallN(m[pkeyCommitIndex], store)
	return ok
}
