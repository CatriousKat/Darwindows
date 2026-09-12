package translation

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const (
	EXCEPTION_BREAKPOINT       = 0x80000003
	EXCEPTION_ACCESS_VIOLATION = 0xc0000005
	EXCEPTION_CONTINUE_SEARCH  = 0
	STD_INPUT_HANDLE           = ^uintptr(9) // uintptr(-10)
	ENABLE_PROCESSED_INPUT     = 0x0001
	ENABLE_LINE_INPUT          = 0x0002
	ENABLE_ECHO_INPUT          = 0x0004
)

var (
	procAddVectoredHandler = modkernel32.NewProc("AddVectoredExceptionHandler")
	procGetStdHandle       = modkernel32.NewProc("GetStdHandle")
	procReadFile           = modkernel32.NewProc("ReadFile")
	procGetConsoleMode     = modkernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = modkernel32.NewProc("SetConsoleMode")
	fdMutex                sync.Mutex
	fdMap                  = map[uintptr]*os.File{
		0: os.Stdin,
		1: os.Stdout,
		2: os.Stderr,
	}
	nextFd uintptr = 3
)

type EXCEPTION_RECORD struct {
	ExceptionCode        uint32
	ExceptionFlags       uint32
	ExceptionRecord      uintptr
	ExceptionAddress     uintptr
	NumberParameters     uint32
	ExceptionInformation [15]uintptr
}

type EXCEPTION_POINTERS struct {
	ExceptionRecord uintptr
	ContextRecord   uintptr
}

func vehHandler(pExceptionPointers uintptr) uintptr {
	if pExceptionPointers == 0 {
		return EXCEPTION_CONTINUE_SEARCH
	}

	pEp := (*EXCEPTION_POINTERS)(unsafe.Pointer(pExceptionPointers))
	if pEp.ExceptionRecord == 0 {
		return EXCEPTION_CONTINUE_SEARCH
	}

	pRec := (*EXCEPTION_RECORD)(unsafe.Pointer(pEp.ExceptionRecord))
	ctxPtr := pEp.ContextRecord

	if pRec.ExceptionCode == EXCEPTION_ACCESS_VIOLATION {
		if ctxPtr != 0 {
			rip := *(*uintptr)(unsafe.Pointer(ctxPtr + 248))
			rax := *(*uintptr)(unsafe.Pointer(ctxPtr + 120))
			rsi := *(*uintptr)(unsafe.Pointer(ctxPtr + 168))
			rdi := *(*uintptr)(unsafe.Pointer(ctxPtr + 176))
			fmt.Printf("[Darwindows VEH] ACCESS VIOLATION at RIP: 0x%x | RAX: 0x%x | RSI: 0x%x | RDI: 0x%x\n", rip, rax, rsi, rdi)
		}
		return EXCEPTION_CONTINUE_SEARCH
	}

	if pRec.ExceptionCode == EXCEPTION_BREAKPOINT {
		if ctxPtr != 0 {
			rax := *(*uintptr)(unsafe.Pointer(ctxPtr + 120))
			rcx := *(*uintptr)(unsafe.Pointer(ctxPtr + 128))
			rdx := *(*uintptr)(unsafe.Pointer(ctxPtr + 136))
			rsi := *(*uintptr)(unsafe.Pointer(ctxPtr + 168))
			rdi := *(*uintptr)(unsafe.Pointer(ctxPtr + 176))
			rip := *(*uintptr)(unsafe.Pointer(ctxPtr + 248))

			if Debug {
				fmt.Printf("[Darwindows VEH] Intercepted breakpoint at RIP: 0x%x, RAX: 0x%x\n", rip, rax)
			}

			sysNum := rax

			switch sysNum {
			case 0x2000001: // sys_exit
				exitCode := int(rdi)
				if Debug {
					fmt.Printf("[Darwindows VEH] Handling sys_exit(code=%d)\n", exitCode)
				}
				os.Exit(exitCode)

			case 0x2000003: // sys_read
				fd := rdi
				buf := rsi
				count := rdx

				if Debug {
					fmt.Printf("[Darwindows VEH] Handling sys_read(fd=%d, buf=0x%x, count=%d)\n", fd, buf, count)
				}

				if fd == 0 {
					hStdin, _, _ := procGetStdHandle.Call(STD_INPUT_HANDLE)
					var mode uint32
					procGetConsoleMode.Call(hStdin, uintptr(unsafe.Pointer(&mode)))
					procSetConsoleMode.Call(hStdin, uintptr(mode|ENABLE_PROCESSED_INPUT|ENABLE_LINE_INPUT|ENABLE_ECHO_INPUT))

					var bytesRead uint32
					data := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(count))
					ret, _, _ := procReadFile.Call(hStdin, uintptr(unsafe.Pointer(&data[0])), uintptr(count), uintptr(unsafe.Pointer(&bytesRead)), 0)
					if ret == 0 {
						rax = 0
					} else {
						rax = uintptr(bytesRead)
					}
				} else {
					fdMutex.Lock()
					f, exists := fdMap[fd]
					fdMutex.Unlock()

					if !exists {
						rax = uintptr(syscall.EBADF)
					} else {
						data := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(count))
						n, err := f.Read(data)
						if err != nil && n == 0 {
							rax = 0
						} else {
							rax = uintptr(n)
						}
					}
				}

				rip += 2
				*(*uintptr)(unsafe.Pointer(ctxPtr + 120)) = rax
				*(*uintptr)(unsafe.Pointer(ctxPtr + 248)) = rip
				return ^uintptr(0)

			case 0x2000004: // sys_write
				fd := rdi
				buf := rsi
				count := rdx

				if Debug {
					fmt.Printf("[Darwindows VEH] Handling sys_write(fd=%d, buf=0x%x, count=%d)\n", fd, buf, count)
				}

				fdMutex.Lock()
				f, exists := fdMap[fd]
				fdMutex.Unlock()

				if !exists {
					rax = uintptr(syscall.EBADF)
				} else {
					data := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(count))
					n, err := f.Write(data)
					if err == nil {
						f.Sync()
						rax = uintptr(n)
					} else {
						rax = 0
					}
				}

				rip += 2
				*(*uintptr)(unsafe.Pointer(ctxPtr + 120)) = rax
				*(*uintptr)(unsafe.Pointer(ctxPtr + 248)) = rip
				return ^uintptr(0)

			case 0x2000005: // sys_open
				pathPtr := rdi
				flags := rsi
				_ = rcx

				path := CStringToString(pathPtr)
				if Debug {
					fmt.Printf("[Darwindows VEH] Handling sys_open(path=%s, flags=0x%x)\n", path, flags)
				}

				openFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0666)
				if err != nil {
					rax = uintptr(syscall.ENOENT)
				} else {
					fdMutex.Lock()
					newFd := nextFd
					nextFd++
					fdMap[newFd] = openFile
					fdMutex.Unlock()
					rax = newFd
				}

				rip += 2
				*(*uintptr)(unsafe.Pointer(ctxPtr + 120)) = rax
				*(*uintptr)(unsafe.Pointer(ctxPtr + 248)) = rip
				return ^uintptr(0)

			case 0x2000006: // sys_close
				fd := rdi
				if Debug {
					fmt.Printf("[Darwindows VEH] Handling sys_close(fd=%d)\n", fd)
				}

				fdMutex.Lock()
				if f, exists := fdMap[fd]; exists && fd > 2 {
					f.Close()
					delete(fdMap, fd)
					rax = 0
				} else {
					rax = uintptr(syscall.EBADF)
				}
				fdMutex.Unlock()

				rip += 2
				*(*uintptr)(unsafe.Pointer(ctxPtr + 120)) = rax
				*(*uintptr)(unsafe.Pointer(ctxPtr + 248)) = rip
				return ^uintptr(0)
			}
		}
	}
	return EXCEPTION_CONTINUE_SEARCH
}

func CStringToString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var bytes []byte
	for i := uintptr(0); ; i++ {
		b := *(*byte)(unsafe.Pointer(ptr + i))
		if b == 0 {
			break
		}
		bytes = append(bytes, b)
	}
	return string(bytes)
}

func RegisterVEH() {
	handlerPtr := syscall.NewCallback(func(p uintptr) uintptr {
		return vehHandler(p)
	})
	procAddVectoredHandler.Call(1, handlerPtr)
}
