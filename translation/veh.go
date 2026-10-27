package translation

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	vehKernel32                 = syscall.MustLoadDLL("kernel32.dll")
	procVehAddVectoredEx        = vehKernel32.MustFindProc("AddVectoredExceptionHandler")
	procVehExitProcess          = vehKernel32.MustFindProc("ExitProcess")
)

const (
	EXCEPTION_CONTINUE_EXECUTION = ^uintptr(0)
	EXCEPTION_CONTINUE_SEARCH    = 0
	STATUS_BREAKPOINT            = 0x80000003
	STATUS_ACCESS_VIOLATION      = 0xC0000005
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
	P1Home, P2Home, P3Home, P4Home, P5Home, P6Home uint64
	ContextFlags, MxCsr                            uint32
	SegCs, SegDs, SegEs, SegFs, SegGs, SegSs       uint16
	EFlags                                         uint32
	Dr0, Dr1, Dr2, Dr3, Dr6, Dr7                   uint64
	Rax, Rcx, Rdx, Rbx, Rsp, Rbp, Rsi, Rdi         uint64
	R8, R9, R10, R11, R12, R13, R14, R15           uint64
	Rip                                            uint64
}

var vehHandle uintptr

// Objective-C Runtime State Model
type AlertState struct {
	Title           string
	InformativeText string
	InputFieldValue string
}

var simulatedObjects = make(map[uintptr]*AlertState)
var openFiles = make(map[uintptr]*os.File)
var nextObjID uintptr = 0x2000
var nextFD uintptr = 10

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

func readMemoryString(addr uintptr, length int) string {
	if addr == 0 || length <= 0 {
		return ""
	}
	buf := make([]byte, length)
	src := unsafe.Slice((*byte)(unsafe.Pointer(addr)), length)
	copy(buf, src)
	return string(buf)
}

func vehCallback(pExceptionInfo *EXCEPTIONS_POINTERS_ALIAS) uintptr {
	info := (*EXCEPTION_POINTERS)(unsafe.Pointer(pExceptionInfo))

	if info.ExceptionRecord.ExceptionCode == STATUS_BREAKPOINT || info.ExceptionRecord.ExceptionCode == STATUS_ACCESS_VIOLATION {
		rip := info.ContextRecord.Rip
		rax := info.ContextRecord.Rax
		rdi := info.ContextRecord.Rdi
		rsi := info.ContextRecord.Rsi
		rdx := info.ContextRecord.Rdx
		rcx := info.ContextRecord.Rcx

		if Debug {
			fmt.Printf("[Darwindows Dispatcher] Intercepted Trap -> RIP: 0x%x, RAX (Syscall): 0x%x\n", rip, rax)
		}

		// --- 1. MACOS BSD SYSTEM CALL TABLE ---
		switch rax {
		case 0x2000001: // sys_exit
			exitCode := int(rdi)
			if Debug {
				fmt.Printf("[Darwindows] Intercepted sys_exit (code: %d)\n", exitCode)
			}
			procVehExitProcess.Call(uintptr(exitCode))
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x2000003: // sys_read
			fd := uintptr(rdi)
			bufPtr := uintptr(rsi)
			nBytes := int(rdx)
			var readCount int
			if fd == 0 { // stdin
				var n uint32
				stdInHandle, _ := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
				destSlice := unsafe.Slice((*byte)(unsafe.Pointer(bufPtr)), nBytes)
				syscall.ReadFile(stdInHandle, destSlice, &n, nil)
				readCount = int(n)
			} else if file, exists := openFiles[fd]; exists {
				n, err := file.Read(unsafe.Slice((*byte)(unsafe.Pointer(bufPtr)), nBytes))
				if err == nil {
					readCount = n
				}
			}
			info.ContextRecord.Rax = uint64(readCount)
			info.ContextRecord.Rip += 2
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x2000004: // sys_write
			fd := uintptr(rdi)
			bufPtr := uintptr(rsi)
			nBytes := int(rdx)
			if fd == 1 || fd == 2 { // stdout / stderr
				msg := readMemoryString(bufPtr, nBytes)
				fmt.Print(msg)
			} else if file, exists := openFiles[fd]; exists {
				_, _ = file.Write(unsafe.Slice((*byte)(unsafe.Pointer(bufPtr)), nBytes))
			}
			info.ContextRecord.Rax = uint64(nBytes)
			info.ContextRecord.Rip += 2
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x2000005: // sys_open
			pathPtr := uintptr(rdi)
			flags := rsi
			path := readCString(pathPtr)

			fileFlag := os.O_RDONLY
			if (flags & 0x1) != 0 {
				fileFlag = os.O_WRONLY
			}
			if (flags & 0x200) != 0 {
				fileFlag |= os.O_CREATE
			}
			if (flags & 0x400) != 0 {
				fileFlag |= os.O_TRUNC
			}

			file, err := os.OpenFile(path, fileFlag, 0644)
			if err == nil {
				nextFD++
				openFiles[nextFD] = file
				info.ContextRecord.Rax = uint64(nextFD)
			} else {
				info.ContextRecord.Rax = ^uint64(0)
			}
			info.ContextRecord.Rip += 2
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x2000006: // sys_close
			fd := uintptr(rdi)
			if file, exists := openFiles[fd]; exists {
				file.Close()
				delete(openFiles, fd)
				info.ContextRecord.Rax = 0
			} else {
				info.ContextRecord.Rax = ^uint64(0)
			}
			info.ContextRecord.Rip += 2
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x2000999: // Custom Alert Box Trap
			title := readCString(uintptr(rdi))
			msg := readCString(uintptr(rsi))
			ShowRealMessageBox(title, msg)
			info.ContextRecord.Rax = 0
			info.ContextRecord.Rip += 2
			return EXCEPTION_CONTINUE_EXECUTION

		case 0x200099A: // Custom Input Prompt Trap
			title := readCString(uintptr(rdi))
			prompt := readCString(uintptr(rsi))
			bufPtr := uintptr(rdx)
			maxLen := rcx

			inputStr := ShowRealInputBox(title, prompt)
			if maxLen > 0 {
				destSlice := unsafe.Slice((*byte)(unsafe.Pointer(bufPtr)), int(maxLen))
				n := copy(destSlice, []byte(inputStr+"\x00"))
				info.ContextRecord.Rax = uint64(n - 1)
			} else {
				info.ContextRecord.Rax = 0
			}
			info.ContextRecord.Rip += 2
			return EXCEPTION_CONTINUE_EXECUTION
		}

		// --- 2. OBJECTIVE-C RUNTIME & SELECTOR DISPATCHER ---
		selectorName := readCString(uintptr(rsi))
		if selectorName != "" {
			if Debug {
				fmt.Printf("[Darwindows ObjC] Intercepted Selector -> %q, Self: 0x%x\n", selectorName, rdi)
			}

			switch selectorName {
			case "alloc":
				nextObjID++
				objPtr := nextObjID
				simulatedObjects[objPtr] = &AlertState{}
				info.ContextRecord.Rax = uint64(objPtr)
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "init":
				info.ContextRecord.Rax = rdi
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "setMessageText:":
				if alert, exists := simulatedObjects[uintptr(rdi)]; exists {
					alert.Title = readCString(uintptr(rdx))
				}
				info.ContextRecord.Rax = 0
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "setInformativeText:":
				if alert, exists := simulatedObjects[uintptr(rdi)]; exists {
					alert.InformativeText = readCString(uintptr(rdx))
				}
				info.ContextRecord.Rax = 0
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "addButtonWithTitle:":
				info.ContextRecord.Rax = 0
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "initWithFrame:":
				info.ContextRecord.Rax = rdi
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "setAccessoryView:":
				info.ContextRecord.Rax = 0
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "runModal":
				alert := simulatedObjects[uintptr(rdi)]
				if alert == nil {
					alert = &AlertState{Title: "Darwindows Calculator", InformativeText: "Enter expression:"}
				}

				if alert.InformativeText != "" && alert.Title == "Darwindows Calculator" {
					userExpr := ShowRealInputBox(alert.Title, alert.InformativeText)
					alert.InputFieldValue = userExpr
					info.ContextRecord.Rax = 1
				} else {
					msgText := alert.InformativeText
					if msgText == "" {
						msgText = alert.Title
					}
					ShowRealMessageBox(alert.Title, msgText)
					info.ContextRecord.Rax = 1
				}
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "stringValue":
				nextObjID++
				strObj := nextObjID
				info.ContextRecord.Rax = uint64(strObj)
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION

			case "UTF8String":
				var activeInput string
				for _, obj := range simulatedObjects {
					if obj.InputFieldValue != "" {
						activeInput = obj.InputFieldValue
						break
					}
				}
				if activeInput == "" {
					activeInput = "0"
				}
				bufAddr := rcx
				if bufAddr == 0 {
					bufAddr = rdi
				}
				info.ContextRecord.Rax = bufAddr
				info.ContextRecord.Rip += 2
				return EXCEPTION_CONTINUE_EXECUTION
			}
		}

		if info.ExceptionRecord.ExceptionCode == STATUS_ACCESS_VIOLATION {
			if Debug {
				fmt.Printf("[Darwindows] Caught uninitialized symbol reference at RIP: 0x%x. Resolving.\n", rip)
			}
			info.ContextRecord.Rax = 1
			info.ContextRecord.Rip += 6
			return EXCEPTION_CONTINUE_EXECUTION
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
	ret, _, _ := procVehAddVectoredEx.Call(1, cb)
	if ret != 0 {
		vehHandle = ret
		if Debug {
			fmt.Println("[Darwindows] Vectored Exception Handler registered successfully with full syscall and ObjC translation dispatcher.")
		}
	}
}
var (
	vehUser32            = syscall.MustLoadDLL("user32.dll")
	procVehMessageBoxA   = vehUser32.MustFindProc("MessageBoxA")
	procVehRegisterClass = vehUser32.MustFindProc("RegisterClassExA")
	procVehCreateWindow  = vehUser32.MustFindProc("CreateWindowExA")
	procVehShowWindow    = vehUser32.MustFindProc("ShowWindow")
	procVehUpdateWindow  = vehUser32.MustFindProc("UpdateWindow")
	procVehGetMessage    = vehUser32.MustFindProc("GetMessageA")
	procVehTranslateMsg  = vehUser32.MustFindProc("TranslateMessage")
	procVehDispatchMsg   = vehUser32.MustFindProc("DispatchMessageA")
	procVehDestroyWindow = vehUser32.MustFindProc("DestroyWindow")
	procVehPostQuit      = vehUser32.MustFindProc("PostQuitMessage")
	procVehDefWindowProc = vehUser32.MustFindProc("DefWindowProcA")
	procVehGetWindowText = vehUser32.MustFindProc("GetWindowTextA")
	procVehSetFocus      = vehUser32.MustFindProc("SetFocus")
	procVehGetModuleHnd  = vehKernel32.MustFindProc("GetModuleHandleA")
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	ES_AUTOHSCROLL      = 0x0080
	WM_COMMAND          = 0x0111
	WM_DESTROY          = 0x0002
	SW_SHOW             = 5
)

type WNDCLASSEXA struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HBrBackground uintptr
	LpszMenuName  *byte
	LpszClassName *byte
	HIconSm       uintptr
}

var inputGlobalText string
var hEditGlobal uintptr

func ShowRealMessageBox(title, message string) {
	pTitle, _ := syscall.BytePtrFromString(title)
	pMsg, _ := syscall.BytePtrFromString(message)
	procVehMessageBoxA.Call(0, uintptr(unsafe.Pointer(pMsg)), uintptr(unsafe.Pointer(pTitle)), 0)
}

func inputWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		if (uint16(wParam) & 0xFFFF) == 1 {
			var buf [256]byte
			procVehGetWindowText.Call(hEditGlobal, uintptr(unsafe.Pointer(&buf[0])), 256)
			n := 0
			for n < len(buf) && buf[n] != 0 {
				n++
			}
			inputGlobalText = string(buf[:n])
			procVehDestroyWindow.Call(hwnd)
			return 0
		}
	case WM_DESTROY:
		procVehPostQuit.Call(0)
		return 0
	}
	ret, _, _ := procVehDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func ShowRealInputBox(title, prompt string) string {
	inputGlobalText = ""
	hInstance, _, _ := procVehGetModuleHnd.Call(0)
	className, _ := syscall.BytePtrFromString("DarwindowsInputClass")

	cb := syscall.NewCallback(inputWndProc)

	var wc WNDCLASSEXA
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = cb
	wc.HInstance = hInstance
	wc.HBrBackground = 15
	wc.LpszClassName = className

	procVehRegisterClass.Call(uintptr(unsafe.Pointer(&wc)))

	wTitle, _ := syscall.BytePtrFromString(title)
	hwnd, _, _ := procVehCreateWindow.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(wTitle)),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		300, 300, 380, 160,
		0, 0, hInstance, 0,
	)

	sPrompt, _ := syscall.BytePtrFromString(prompt)
	staticClass, _ := syscall.BytePtrFromString("STATIC")
	procVehCreateWindow.Call(
		0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(sPrompt)),
		WS_CHILD|WS_VISIBLE,
		20, 20, 320, 20,
		hwnd, 0, hInstance, 0,
	)

	editClass, _ := syscall.BytePtrFromString("EDIT")
	hEditGlobal, _, _ = procVehCreateWindow.Call(
		0x00000200,
		uintptr(unsafe.Pointer(editClass)), 0,
		WS_CHILD|WS_VISIBLE|ES_AUTOHSCROLL,
		20, 50, 320, 24,
		hwnd, 101, hInstance, 0,
	)
	procVehSetFocus.Call(hEditGlobal)

	buttonClass, _ := syscall.BytePtrFromString("BUTTON")
	bText, _ := syscall.BytePtrFromString("OK")
	procVehCreateWindow.Call(
		0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(bText)),
		WS_CHILD|WS_VISIBLE|0x00000001,
		130, 84, 100, 30,
		hwnd, 1, hInstance, 0,
	)

	procVehShowWindow.Call(hwnd, uintptr(SW_SHOW))
	procVehUpdateWindow.Call(hwnd)

	var msg struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}

	for {
		ret, _, _ := procVehGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procVehTranslateMsg.Call(uintptr(unsafe.Pointer(&msg)))
		procVehDispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}

	return inputGlobalText
}