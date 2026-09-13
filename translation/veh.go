package translation

import (
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	EXCEPTION_BREAKPOINT       = 0x80000003
	EXCEPTION_ACCESS_VIOLATION = 0xc0000005
	EXCEPTION_CONTINUE_SEARCH  = 0
	STD_INPUT_HANDLE           = ^uintptr(9)
	ENABLE_PROCESSED_INPUT     = 0x0001
	ENABLE_LINE_INPUT          = 0x0002
	ENABLE_ECHO_INPUT          = 0x0004
	IDC_ARROW                  = 32512
)

var (
	moduser32              = syscall.NewLazyDLL("user32.dll")
	modgdi32               = syscall.NewLazyDLL("gdi32.dll")
	procAddVectoredHandler = modkernel32.NewProc("AddVectoredExceptionHandler")
	procGetStdHandle       = modkernel32.NewProc("GetStdHandle")
	procReadFile           = modkernel32.NewProc("ReadFile")
	procGetConsoleMode     = modkernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = modkernel32.NewProc("SetConsoleMode")
	procGetModuleHandleW   = modkernel32.NewProc("GetModuleHandleW")
	procCreateWindowExW    = moduser32.NewProc("CreateWindowExW")
	procShowWindow         = moduser32.NewProc("ShowWindow")
	procUpdateWindow       = moduser32.NewProc("UpdateWindow")
	procGetMessageW        = moduser32.NewProc("GetMessageW")
	procTranslateMessage   = moduser32.NewProc("TranslateMessage")
	procDispatchMessageW   = moduser32.NewProc("DispatchMessageW")
	procDestroyWindow      = moduser32.NewProc("DestroyWindow")
	procDefWindowProcW     = moduser32.NewProc("DefWindowProcW")
	procRegisterClassExW   = moduser32.NewProc("RegisterClassExW")
	procGetWindowTextW     = moduser32.NewProc("GetWindowTextW")
	procSetFocus           = moduser32.NewProc("SetFocus")
	procCreateSolidBrush   = modgdi32.NewProc("CreateSolidBrush")
	procPostQuitMessage    = moduser32.NewProc("PostQuitMessage")
	procLoadCursorW        = moduser32.NewProc("LoadCursorW")

	fdMutex sync.Mutex
	fdMap   = map[uintptr]*os.File{
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

var (
	inputResultBuffer string
	dialogHasInput    bool
	hEditGlobal       uintptr
	dialogDone        = make(chan struct{})
	dialogMutex       sync.Mutex
	classRegistered   bool
)

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

			sysNum := rax

			switch sysNum {
			case 0x2000001: // sys_exit
				os.Exit(int(rdi))

			case 0x2000003: // sys_read
				fd := rdi
				buf := rsi
				count := rdx

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
				path := CStringToString(pathPtr)
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

			case 0x2000999, 0x200099A: // Message Box / Input Prompt
				titlePtr := rdi
				msgPtr := rsi
				outBufPtr := rdx
				maxLen := rcx

				title := CStringToString(titlePtr)
				msg := CStringToString(msgPtr)
				hasInput := (sysNum == 0x200099A)

				println("[DEBUG] Showing dialog:", title, "|", msg)

				dialogMutex.Lock()
				go showCocoaDialogWindow(title, msg, hasInput)
				<-dialogDone
				resultText := inputResultBuffer
				dialogMutex.Unlock()

				if hasInput && outBufPtr != 0 && maxLen > 0 {
					destSlice := unsafe.Slice((*byte)(unsafe.Pointer(outBufPtr)), int(maxLen))
					copyBytes := []byte(resultText)
					if len(copyBytes) > int(maxLen)-1 {
						copyBytes = copyBytes[:int(maxLen)-1]
					}
					copy(destSlice, copyBytes)
					destSlice[len(copyBytes)] = 0
					rax = uintptr(len(copyBytes))
				} else {
					rax = 0
				}

				rip += 2
				*(*uintptr)(unsafe.Pointer(ctxPtr + 120)) = rax
				*(*uintptr)(unsafe.Pointer(ctxPtr + 248)) = rip
				return ^uintptr(0)
			}
		}
	}
	return EXCEPTION_CONTINUE_SEARCH
}

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

func cocoaWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	const WM_COMMAND = 0x0111
	const WM_CLOSE = 0x0010
	const BN_CLICKED = 0

	if msg == WM_COMMAND {
		code := uint32(wParam >> 16)
		id := uint16(wParam & 0xFFFF)
		if id == 1 && code == BN_CLICKED {
			if dialogHasInput && hEditGlobal != 0 {
				var buf [256]uint16
				procGetWindowTextW.Call(hEditGlobal, uintptr(unsafe.Pointer(&buf[0])), 256)
				inputResultBuffer = syscall.UTF16ToString(buf[:])
			}
			procDestroyWindow.Call(hwnd)
			procPostQuitMessage.Call(0)
		}
	} else if msg == WM_CLOSE {
		procDestroyWindow.Call(hwnd)
		procPostQuitMessage.Call(0)
	} else {
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return ret
	}
	return 0
}

func showCocoaDialogWindow(title, prompt string, hasInput bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	inputResultBuffer = ""
	dialogHasInput = hasInput
	className, _ := syscall.UTF16PtrFromString("DarwindowsCocoaSystemDialog")

	hInst, _, _ := procGetModuleHandleW.Call(0)
	hBrush, _, _ := procCreateSolidBrush.Call(uintptr(245 | (245 << 8) | (247 << 16)))
	hCursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)

	if !classRegistered {
		var wc WNDCLASSEX
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		wc.LpfnWndProc = syscall.NewCallback(cocoaWndProc)
		wc.HInstance = hInst
		wc.HbrBackground = hBrush
		wc.HCursor = hCursor
		wc.LpszClassName = className
		ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if ret != 0 {
			classRegistered = true
			println("[DEBUG] RegisterClassExW succeeded, atom:", ret)
		} else {
			println("[DEBUG] RegisterClassExW failed:", err.Error())
		}
	}

	tW, _ := syscall.UTF16PtrFromString(title)
	pW, _ := syscall.UTF16PtrFromString(prompt)

	height := uintptr(160)
	if hasInput {
		height = 200
	}

	hwnd, _, err := procCreateWindowExW.Call(
		0x00000200, // WS_EX_TOOLWINDOW
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(tW)),
		0x90CA0000, // WS_POPUP | WS_CAPTION | WS_SYSMENU | WS_VISIBLE
		500, 350, 400, height,
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		println("[DEBUG] CreateWindowExW failed:", err.Error())
		dialogDone <- struct{}{}
		return
	}

	classNameStatic, _ := syscall.UTF16PtrFromString("STATIC")
	procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(classNameStatic)), uintptr(unsafe.Pointer(pW)),
		0x50020000, // WS_CHILD | WS_VISIBLE | SS_LEFT
		25, 25, 340, 40,
		hwnd, 0, hInst, 0,
	)

	if hasInput {
		classNameEdit, _ := syscall.UTF16PtrFromString("EDIT")
		emptyStr, _ := syscall.UTF16PtrFromString("")
		hEditGlobal, _, _ = procCreateWindowExW.Call(
			0x00000200, // WS_EX_CLIENTEDGE
			uintptr(unsafe.Pointer(classNameEdit)), uintptr(unsafe.Pointer(emptyStr)),
			0x50810080, // WS_CHILD | WS_VISIBLE | WS_TABSTOP | ES_AUTOHSCROLL
			25, 75, 335, 28,
			hwnd, 0, hInst, 0,
		)
		procSetFocus.Call(hEditGlobal)
	}

	classNameButton, _ := syscall.UTF16PtrFromString("BUTTON")
	btnText, _ := syscall.UTF16PtrFromString("OK")
	btnY := uintptr(85)
	if hasInput {
		btnY = 120
	}

	procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(classNameButton)), uintptr(unsafe.Pointer(btnText)),
		0x50010001, // WS_CHILD | WS_VISIBLE | BS_DEFPUSHBUTTON
		260, btnY, 100, 32,
		hwnd, 1, hInst, 0,
	)

	procShowWindow.Call(hwnd, 5)
	procUpdateWindow.Call(hwnd)

	var msg struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}

	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	dialogDone <- struct{}{}
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