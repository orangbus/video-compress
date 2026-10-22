package video

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// openRegKey 打开只读注册表键
func openRegKey(path string) (registry.Key, error) {
	return registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
}

// newLazyDLL 返回 kernel32 等系统 DLL（仅 Windows 使用）
func newLazyDLL(name string) *syscall.LazyDLL {
	return syscall.NewLazyDLL(name)
}

// utf16PtrToString 兼容辅助：将 *uint16 转为 string
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var b []uint16
	for *p != 0 {
		b = append(b, *p)
		p = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + unsafe.Sizeof(*p)))
	}
	return syscall.UTF16ToString(b)
}
