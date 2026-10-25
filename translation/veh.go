package translation

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32DLL, _                       = syscall.LoadDLL("user32.dll")
	procAddVectoredExceptionHandler, _ = kernel32DLL.FindProc("AddVectoredExceptionHandler")
	procRemoveVectoredExceptionHandler,_= kernel32DLL.FindProc("RemoveVectoredExceptionHandler")
	procMessageBoxA, _                 = user32DLL.FindProc("MessageBoxA")
	procExitProcess, _                 = kernel32DLL.FindProc("ExitProcess")
	procCreateWindowExA, _             = user32DLL.FindProc("CreateWindowExA")
	procShowWindow, _                  = user32DLL.FindProc("ShowWindow")
	procUpdateWindow, _                = user32DLL.FindProc("UpdateWindow")
	procGetMessageA, _                 = user32DLL.FindProc("GetMessageA")
	procTranslateMessage, _            = user32DLL.FindProc("TranslateMessage")
	procDispatchMessageA, _            = user32DLL.FindProc("DispatchMessageA")
	procPostQuitMessage, _             = user32DLL.FindProc("PostQuitMessage")
	procRegisterClassA, _              = user32DLL.FindProc("RegisterClassA")
	procDefWindowProcA, _              = user32DLL.FindProc("DefWindowProcA")
	procGetWindowTextA, _              = user32DLL.FindProc("GetWindowTextA")
	procDestroyWindow, _               = user32DLL.FindProc("DestroyWindow")
	procGetModuleHandleA, _            = kernel32DLL.FindProc("GetModuleHandleA")
)

const (
	EXCEPTION_CONTINUE_EXECUTION = ^uintptr(0)
	EXCEPTION_CONTINUE_SEARCH    = 0
	STATUS_BREAKPOINT            = 0x80000003
)

type EXCEPTION_POINTERS struct {
	ExceptionRecord *EXCEPTION_RECORD
	ContextRecord   *CONTEXT
}

type EXCEPTION_RECORD struct {
	ExceptionCode        uint32
	ExceptionFlags       uint32
	ExceptionRecord      *EXCEPTION_RECORD
	ExceptionAddress     uintptr
	NumberParameters     uint32
	ExceptionInformation [15]uintptr
}

type CONTEXT struct {
	P1Home       uint64
	P2Home       uint64
	P3Home       uint64
	P4Home       uint64
	P5Home       uint64
	P6Home       uint64
	ContextFlags uint32
	MxCsr        uint32
	SegCs        uint16
	SegDs        uint16
	SegEs        uint16
	SegFs        uint16
	SegGs        uint16
	SegSs        uint16
	EFlags       uint32
	Dr0          uint64
	Dr1          uint64
	Dr2          uint64
	Dr3          uint64
	Dr6          uint64
	Dr7          uint64
	Rax          uint64
	Rcx          uint64
	Rdx          uint64
	Rbx          uint64
	Rsp          uint64
	Rbp          uint64
	Rsi          uint64
	Rdi          uint64
	R8           uint64
	R9           uint64
	R10          uint64
	R11          uint64
	R12          uint64
	R13          uint64
	R14          uint64
	R15          uint64
	Rip          uint64
}

type WNDCLASSA struct {
	Style         uint32
	_             [4]byte
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *byte
	LpszClassName *byte
}

var vehHandle uintptr

func readCString(addr uintptr) string {
	if addr == 0 {
		return ""
	}
	var buf []byte
	for i := uintptr(0); i < 4096; i++ {
		b := *(*byte)(unsafe.Pointer(addr + i))
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}

var (
	inputResult     string
	hEditCtrl       uintptr
	classRegistered bool
)

func inputDlgWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case 0x0111: // WM_COMMAND
		if LOWORD(uint32(wParam)) == 1 { // OK button ID
			var buf [256]byte
			ret, _, _ := procGetWindowTextA.Call(hEditCtrl, uintptr(unsafe.Pointer(&buf[0])), 256)
			if ret > 0 {
				inputResult = string(buf[:ret])
			} else {
				inputResult = ""
			}
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case 0x0002: // WM_DESTROY
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcA.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func LOWORD(val uint32) uint16 {
	return uint16(val & 0xFFFF)
}

func showGUIInputBox(title, prompt string) string {
	inputResult = ""
	className, _ := syscall.BytePtrFromString("DarwindowsWin32InputBox")
	hInst, _, _ := procGetModuleHandleA.Call(0)

	if !classRegistered {
		wndClass := WNDCLASSA{
			Style:         0,
			LpfnWndProc:   syscall.NewCallback(inputDlgWndProc),
			CbClsExtra:    0,
			CbWndExtra:    0,
			HInstance:     hInst,
			HCursor:       0,
			HbrBackground: 15, // COLOR_WINDOW + 1
			LpszClassName: className,
		}
		procRegisterClassA.Call(uintptr(unsafe.Pointer(&wndClass)))
		classRegistered = true
	}

	pTitle, _ := syscall.BytePtrFromString(title)
	hwnd, _, _ := procCreateWindowExA.Call(
		0x00000100, // WS_EX_DLGMODALFRAME
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(pTitle)),
		0x94CA0000, // WS_VISIBLE | WS_POPUP | WS_CAPTION | WS_SYSMENU
		400, 300, 360, 150,
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		return ""
	}

	// Create Static Prompt Label
	pPrompt, _ := syscall.BytePtrFromString(prompt)
	clsStatic, _ := syscall.BytePtrFromString("STATIC")
	procCreateWindowExA.Call(0, uintptr(unsafe.Pointer(clsStatic)), uintptr(unsafe.Pointer(pPrompt)), 0x50000000, 20, 15, 300, 20, hwnd, 0, hInst, 0)

	// Create Edit Control (Input Text Box)
	clsEdit, _ := syscall.BytePtrFromString("EDIT")
	hEdit, _, _ := procCreateWindowExA.Call(0x00000200, uintptr(unsafe.Pointer(clsEdit)), 0, 0x50010080, 20, 45, 300, 25, hwnd, 101, hInst, 0)
	hEditCtrl = hEdit

	// Create OK Button
	clsButton, _ := syscall.BytePtrFromString("BUTTON")
	btnOK, _ := syscall.BytePtrFromString("OK")
	procCreateWindowExA.Call(0, uintptr(unsafe.Pointer(clsButton)), uintptr(unsafe.Pointer(btnOK)), 0x50010000, 230, 80, 90, 30, hwnd, 1, hInst, 0)

	procShowWindow.Call(hwnd, 5) // SW_SHOW
	procUpdateWindow.Call(hwnd)

	// Win32 Message Loop
	var msg struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}
	for {
		ret, _, _ := procGetMessageA.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageA.Call(uintptr(unsafe.Pointer(&msg)))
	}

	return inputResult
}

func vehCallback(pExceptionInfo *EXCEPTIONS_POINTERS_ALIAS) uintptr {
	info := (*EXCEPTION_POINTERS)(unsafe.Pointer(pExceptionInfo))

	if info.ExceptionRecord.ExceptionCode == STATUS_BREAKPOINT {
		rip := info.ContextRecord.Rip
		rax := info.ContextRecord.Rax

		if Debug {
			fmt.Printf("[Darwindows VEH] Caught breakpoint trap at RIP: 0x%x, RAX: 0x%x\n", rip, rax)
		}

		switch rax {
		case 0x2000997: // Get macOS version info
			bufPtr := info.ContextRecord.Rcx
			maxLen := info.ContextRecord.Rdx

			osStr := "macOS 10.15 Catalina"
			if maxLen > 0 {
				destSlice := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(bufPtr))), maxLen)
				n := copy(destSlice, []byte(osStr+"\x00"))
				info.ContextRecord.Rax = uint64(n)
			} else {
				info.ContextRecord.Rax = 0
			}
			info.ContextRecord.Rip += 1
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x2000999: // Message Box
			titlePtr := info.ContextRecord.Rdi
			msgPtr := info.ContextRecord.Rsi

			title := readCString(uintptr(titlePtr))
			msg := readCString(uintptr(msgPtr))

			if Debug {
				fmt.Printf("[Darwindows GUI] MessageBox -> Title: %q, Msg: %q\n", title, msg)
			}

			pTitle, _ := syscall.BytePtrFromString(title)
			pMsg, _ := syscall.BytePtrFromString(msg)
			procMessageBoxA.Call(0, uintptr(unsafe.Pointer(pMsg)), uintptr(unsafe.Pointer(pTitle)), 0)

			info.ContextRecord.Rax = 0
			info.ContextRecord.Rip += 1
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x200099A: // Native Win32 GUI Input Box
			titlePtr := info.ContextRecord.Rdi
			promptPtr := info.ContextRecord.Rsi
			bufPtr := info.ContextRecord.Rdx
			maxLen := info.ContextRecord.Rcx

			title := readCString(uintptr(titlePtr))
			prompt := readCString(uintptr(promptPtr))

			if Debug {
				fmt.Printf("[Darwindows GUI] InputBox -> Title: %q, Prompt: %q\n", title, prompt)
			}

			inputStr := showGUIInputBox(title, prompt)

			if maxLen > 0 {
				destSlice := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(bufPtr))), maxLen)
				n := copy(destSlice, []byte(inputStr+"\x00"))
				info.ContextRecord.Rax = uint64(n - 1)
			} else {
				info.ContextRecord.Rax = 0
			}

			info.ContextRecord.Rip += 1
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x2000001: // macOS sys_exit
			exitCode := info.ContextRecord.Rdi
			if Debug {
				fmt.Printf("[Darwindows] Intercepted sys_exit (code: %d). Terminating.\n", exitCode)
			}
			procExitProcess.Call(uintptr(exitCode))
			return EXCEPTION_CONTINUE_EXECUTION

		default:
			if Debug {
				fmt.Printf("[Darwindows ERROR] Unhandled syscall/trap RAX: 0x%x at RIP: 0x%x\n", rax, rip)
			}
		}
	}

	return EXCEPTION_CONTINUE_SEARCH
}

type EXCEPTIONS_POINTERS_ALIAS = struct {
	ExceptionRecord *EXCEPTION_RECORD
	ContextRecord   *CONTEXT
}

func RegisterVEH() {
	if vehHandle != 0 {
		return
	}
	cb := syscall.NewCallback(func(pInfo *EXCEPTIONS_POINTERS_ALIAS) uintptr {
		return vehCallback(pInfo)
	})

	ret, _, _ := procAddVectoredExceptionHandler.Call(1, cb)
	if ret != 0 {
		vehHandle = ret
		if Debug {
			fmt.Println("[Darwindows] Vectored Exception Handler registered successfully.")
		}
	}
}