package translation

import (
	"debug/macho"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

// Debug controls whether verbose loader and VEH logs are printed to stdout
var Debug bool

var (
	modkernel32             = syscall.NewLazyDLL("kernel32.dll")
	procVirtualAlloc        = modkernel32.NewProc("VirtualAlloc")
	procCreateThread        = modkernel32.NewProc("CreateThread")
	procWaitForSingleObject = modkernel32.NewProc("WaitForSingleObject")
)

const (
	MEM_RESERVE            = 0x00002000
	MEM_COMMIT             = 0x00001000
	PAGE_EXECUTE_READWRITE = 0x40
	INFINITE               = 0xFFFFFFFF
)

func virtualAlloc(addr uintptr, size uintptr, allocationType uint32, protect uint32) (uintptr, error) {
	ret, _, err := procVirtualAlloc.Call(addr, size, uintptr(allocationType), uintptr(protect))
	if ret == 0 {
		return 0, err
	}
	return ret, nil
}

// RunMachO parses, maps, patches, and executes an x86_64 Mach-O binary
func RunMachO(filePath string) error {
	f, err := macho.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open Mach-O file: %w", err)
	}
	defer f.Close()

	if f.Cpu != macho.CpuAmd64 {
		return fmt.Errorf("unsupported CPU architecture: only x86_64 is supported (got %v)", f.Cpu)
	}

	RegisterVEH()

	var entryPoint uintptr
	var firstSegmentAddr uintptr
	segmentMemMap := make(map[string]uintptr)

	for _, load := range f.Loads {
		seg, ok := load.(*macho.Segment)
		if !ok || seg.Memsz == 0 {
			continue
		}

		segName := strings.TrimSpace(seg.Name)

		if segName == "__PAGEZERO" {
			if Debug {
				fmt.Println("[Darwindows] Skipping __PAGEZERO segment")
			}
			continue
		}

		if Debug {
			fmt.Printf("[Darwindows] Mapping segment %s (requested addr: 0x%x, size: 0x%x)\n", segName, seg.Addr, seg.Memsz)
		}

		mem, err := virtualAlloc(0, uintptr(seg.Memsz), MEM_RESERVE|MEM_COMMIT, PAGE_EXECUTE_READWRITE)
		if err != nil {
			return fmt.Errorf("VirtualAlloc failed for segment %s: %w", segName, err)
		}

		segmentMemMap[segName] = mem

		if firstSegmentAddr == 0 {
			firstSegmentAddr = mem
		}

		data, err := seg.Data()
		if err != nil {
			return fmt.Errorf("failed to read data for segment %s: %w", segName, err)
		}

		if len(data) > 0 {
			destSlice := unsafe.Slice((*byte)(unsafe.Pointer(mem)), seg.Memsz)
			copy(destSlice, data)

			for i := 0; i < len(destSlice)-1; i++ {
				if destSlice[i] == 0x0F && destSlice[i+1] == 0x05 {
					destSlice[i] = 0xCC   // INT 3
					destSlice[i+1] = 0x90 // NOP
				}
			}
		}
	}

	if sec := f.Section("__text"); sec != nil {
		if textSegMem, ok := segmentMemMap["__TEXT"]; ok {
			if seg := f.Segment("__TEXT"); seg != nil {
				sectionOffset := sec.Addr - seg.Addr
				entryPoint = textSegMem + uintptr(sectionOffset)
			}
		}
	}

	if entryPoint == 0 {
		if textSegMem, ok := segmentMemMap["__TEXT"]; ok {
			entryPoint = textSegMem
		} else {
			entryPoint = firstSegmentAddr
		}
	}

	if Debug {
		fmt.Printf("[Darwindows] Executing binary via native thread at entry point: 0x%x\n", entryPoint)
	}

	hThread, _, err := procCreateThread.Call(0, 0, entryPoint, 0, 0, 0)
	if hThread == 0 {
		return fmt.Errorf("CreateThread failed: %w", err)
	}

	procWaitForSingleObject.Call(hThread, INFINITE)

	return nil
}
