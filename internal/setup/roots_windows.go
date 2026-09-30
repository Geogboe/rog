//go:build windows

package setup

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	setupKernel32         = syscall.NewLazyDLL("kernel32.dll")
	setupGetLogicalDrives = setupKernel32.NewProc("GetLogicalDrives")
	setupGetDriveType     = setupKernel32.NewProc("GetDriveTypeW")
)

func LocalSearchRoots() []SearchRoot {
	mask, _, _ := setupGetLogicalDrives.Call()
	var roots []SearchRoot
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		path := fmt.Sprintf("%c:\\", 'A'+i)
		p, _ := syscall.UTF16PtrFromString(path)
		typeID, _, _ := setupGetDriveType.Call(uintptr(unsafe.Pointer(p)))
		if typeID == 3 {
			roots = append(roots, SearchRoot{Name: fmt.Sprintf("drive-%c", 'A'+i), Path: path})
		}
	}
	return roots
}
