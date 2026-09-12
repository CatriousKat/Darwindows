package translation

import (
	"debug/macho"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

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

	var minAddr uint64 = ^uint64(0)
	var maxAddr uint64 = 0

	for _, load := range f.Loads {
		seg, ok := load.(*macho.Segment)
		if !ok || seg.Memsz == 0 {
			continue
		}
		segName := strings.TrimSpace(seg.Name)
		if segName == "__PAGEZERO" {
			continue
		}
		if seg.Addr < minAddr {
			minAddr = seg.Addr
		}
		if seg.Addr+seg.Memsz > maxAddr {
			maxAddr = seg.Addr + seg.Memsz
		}
	}

	totalSize := uintptr(maxAddr - minAddr)
	imageBase, err := virtualAlloc(0, totalSize, MEM_RESERVE|MEM_COMMIT, PAGE_EXECUTE_READWRITE)
	if err != nil {
		return fmt.Errorf("VirtualAlloc failed for total image span: %w", err)
	}

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

		segDest := imageBase + uintptr(seg.Addr-minAddr)

		if Debug {
			fmt.Printf("[Darwindows] Mapping segment %s at offset 0x%x (size: 0x%x)\n", segName, seg.Addr-minAddr, seg.Memsz)
		}

		data, err := seg.Data()
		if err != nil {
			return fmt.Errorf("failed to read data for segment %s: %w", segName, err)
		}

		if len(data) > 0 {
			destSlice := unsafe.Slice((*byte)(unsafe.Pointer(segDest)), seg.Memsz)
			copy(destSlice, data)

			patchCount := 0
			for i := 0; i < len(data)-1; i++ {
				if destSlice[i] == 0x0F && destSlice[i+1] == 0x05 {
					destSlice[i] = 0xCC   // INT 3
					destSlice[i+1] = 0x90 // NOP
					patchCount++
				}
			}
			if Debug {
				fmt.Printf("[Darwindows] Patched %d syscall instruction(s) in segment %s\n", patchCount, segName)
			}
		}
	}

	var entryPoint uintptr
	if f.Symtab != nil {
		for _, sym := range f.Symtab.Syms {
			if sym.Name == "_main" {
				entryPoint = imageBase + uintptr(sym.Value-minAddr)
				break
			}
		}
	}

	if entryPoint == 0 {
		if sec := f.Section("__text"); sec != nil {
			entryPoint = imageBase + uintptr(sec.Addr-minAddr)
		} else {
			entryPoint = imageBase
		}
	}

	if Debug {
		codeBytes := unsafe.Slice((*byte)(unsafe.Pointer(entryPoint)), 64)
		fmt.Printf("[Darwindows] Executing binary via native thread at entry point: 0x%x\n", entryPoint)
		fmt.Printf("[Darwindows] First 64 bytes at entry point: %x\n", codeBytes)
	}

	hThread, _, err := procCreateThread.Call(0, 0, entryPoint, 0, 0, 0)
	if hThread == 0 {
		return fmt.Errorf("CreateThread failed: %w", err)
	}

	procWaitForSingleObject.Call(hThread, INFINITE)

	return nil
}
