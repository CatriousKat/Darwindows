package translation

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	EXCEPTION_BREAKPOINT      = 0x80000003
	EXCEPTION_CONTINUE_SEARCH = 0
)

var (
	procAddVectoredHandler = modkernel32.NewProc("AddVectoredExceptionHandler")
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
	if pRec.ExceptionCode == EXCEPTION_BREAKPOINT {
		ctxPtr := pEp.ContextRecord
		if ctxPtr != 0 {
			rax := *(*uintptr)(unsafe.Pointer(ctxPtr + 120))
			rdx := *(*uintptr)(unsafe.Pointer(ctxPtr + 136))
			rsi := *(*uintptr)(unsafe.Pointer(ctxPtr + 168))
			rdi := *(*uintptr)(unsafe.Pointer(ctxPtr + 176))
			rip := *(*uintptr)(unsafe.Pointer(ctxPtr + 248))

			if Debug {
				fmt.Printf("[Darwindows VEH] Intercepted breakpoint at RIP: 0x%x, RAX: 0x%x\n", rip, rax)
			}

			sysNum := rax

			switch sysNum {
			case 0x2000004: // macOS sys_write
				fd := rdi
				buf := rsi
				count := rdx

				if Debug {
					fmt.Printf("[Darwindows VEH] Handling sys_write(fd=%d, buf=0x%x, count=%d)\n", fd, buf, count)
				}

				if fd == 1 || fd == 2 {
					data := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(count))
					if fd == 1 {
						os.Stdout.Write(data)
						os.Stdout.Sync()
					} else {
						os.Stderr.Write(data)
						os.Stderr.Sync()
					}
					rax = count
				} else {
					rax = 0
				}

				rip += 2

				*(*uintptr)(unsafe.Pointer(ctxPtr + 120)) = rax
				*(*uintptr)(unsafe.Pointer(ctxPtr + 248)) = rip

				return ^uintptr(0)

			case 0x2000001: // macOS sys_exit
				exitCode := int(rdi)
				if Debug {
					fmt.Printf("[Darwindows VEH] Handling sys_exit(code=%d)\n", exitCode)
				}
				os.Exit(exitCode)
			}
		}
	}
	return EXCEPTION_CONTINUE_SEARCH
}

func RegisterVEH() {
	handlerPtr := syscall.NewCallback(func(p uintptr) uintptr {
		return vehHandler(p)
	})
	procAddVectoredHandler.Call(1, handlerPtr)
}
